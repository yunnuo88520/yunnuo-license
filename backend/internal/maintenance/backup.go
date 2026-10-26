// Package maintenance provides encrypted, independently restorable backups.
package maintenance

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"filippo.io/age"
	"fmt"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/config"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/filelock"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Format    int              `json:"format"`
	Driver    string           `json:"driver"`
	CreatedAt time.Time        `json:"created_at"`
	Files     map[string]Entry `json:"files"`
}

var safeRelease = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func privateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("private file must be a regular file with mode 0600")
	}
	return nil
}
func fileEntry(path string) (Entry, error) {
	f, e := os.Open(path)
	if e != nil {
		return Entry{}, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	return Entry{n, hex.EncodeToString(h.Sum(nil))}, e
}
func copyFile(from, to string) error {
	info, e := os.Lstat(from)
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() {
		return errors.New("backup refuses non-regular files")
	}
	in, e := os.Open(from)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	return errors.Join(e, out.Close())
}

func Backup(ctx context.Context, cfg config.Config, recipient, output string) (Manifest, error) {
	m := Manifest{Format: 1, Driver: cfg.DatabaseDriver, CreatedAt: time.Now().UTC(), Files: map[string]Entry{}}
	r, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		return m, err
	}
	if _, err = os.Lstat(output); !os.IsNotExist(err) {
		return m, errors.New("backup output must not already exist")
	}
	if err = os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return m, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".backup-")
	if err != nil {
		return m, err
	}
	defer os.RemoveAll(stage)
	if err = os.MkdirAll(cfg.ReleaseDir, 0700); err != nil {
		return m, err
	}
	unlock, err := filelock.Acquire(ctx, cfg.ReleaseDir+".lock", true)
	if err != nil {
		return m, err
	}
	defer unlock()
	st, err := store.Open(ctx, cfg.DatabaseDriver, cfg.Database)
	if err != nil {
		return m, err
	}
	defer st.Close()
	unlockSchema, err := st.LockSchema(ctx)
	if err != nil {
		return m, err
	}
	defer unlockSchema()
	database := "database.sql"
	if cfg.DatabaseDriver == "sqlite" {
		database = "database.sqlite"
	}
	if err = st.SnapshotDatabase(ctx, filepath.Join(stage, database)); err != nil {
		return m, err
	}
	// Package metadata and files must be read from the SAME DB snapshot. Upload,
	// publish and delete all take the exclusive lock held against this backup.
	snap := st
	if cfg.DatabaseDriver == "sqlite" {
		snap, err = store.Open(ctx, "sqlite", "file:"+filepath.Join(stage, database))
		if err != nil {
			return m, err
		}
		defer snap.Close()
	}
	// No migration writes are made to a source or snapshot during backup.
	releases, err := snap.BackupReleases(ctx)
	if err != nil {
		return m, err
	}
	if err = os.Mkdir(filepath.Join(stage, "releases"), 0700); err != nil {
		return m, err
	}
	for _, release := range releases {
		if !safeRelease.MatchString(release.ID) {
			return m, errors.New("invalid stored release ID")
		}
		target := filepath.Join(stage, "releases", release.ID)
		if err = copyFile(filepath.Join(cfg.ReleaseDir, release.ID), target); err != nil {
			return m, fmt.Errorf("release %s: %w", release.ID, err)
		}
		entry, e := fileEntry(target)
		if e != nil {
			return m, e
		}
		if entry.Size != release.Size || entry.SHA256 != release.SHA256 {
			return m, fmt.Errorf("release %s integrity failed", release.ID)
		}
		m.Files["releases/"+release.ID] = entry
	}
	effective := config.DatabaseConfig{Driver: cfg.DatabaseDriver, DSN: cfg.Database, AdminPath: cfg.AdminPath, CardPepper: string(cfg.CardPepper), DataKey: hex.EncodeToString(cfg.DataKey)}
	raw, _ := json.MarshalIndent(effective, "", "  ")
	if err = os.WriteFile(filepath.Join(stage, "config.json"), raw, 0600); err != nil {
		return m, err
	}
	for _, name := range []string{database, "config.json"} {
		entry, e := fileEntry(filepath.Join(stage, name))
		if e != nil {
			return m, e
		}
		m.Files[name] = entry
	}
	raw, _ = json.MarshalIndent(m, "", "  ")
	if err = os.WriteFile(filepath.Join(stage, "manifest.json"), raw, 0600); err != nil {
		return m, err
	}
	out, err := os.OpenFile(filepath.Join(stage, "archive.age"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return m, err
	}
	encrypted, err := age.Encrypt(out, r)
	if err != nil {
		out.Close()
		return m, err
	}
	zipped := gzip.NewWriter(encrypted)
	archive := tar.NewWriter(zipped)
	names := []string{"manifest.json"}
	for name := range m.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			break
		}
		info, e := os.Stat(filepath.Join(stage, name))
		if e != nil {
			err = e
			break
		}
		if err = archive.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: info.Size(), ModTime: m.CreatedAt, Typeflag: tar.TypeReg}); err != nil {
			break
		}
		f, e := os.Open(filepath.Join(stage, name))
		if e != nil {
			err = e
			break
		}
		_, err = io.Copy(archive, f)
		f.Close()
		if err != nil {
			break
		}
	}
	err = errors.Join(err, archive.Close(), zipped.Close(), encrypted.Close(), out.Sync(), out.Close())
	if err != nil {
		return m, err
	}
	// Hard-link provides atomic no-overwrite publication of the encrypted file.
	if err = os.Link(filepath.Join(stage, "archive.age"), output); err != nil {
		return m, err
	}
	return m, nil
}

type RestoreOptions struct {
	Input, IdentityFile, Destination, MySQLDSNFile string
	MaxBytes                                       int64
}

func Restore(ctx context.Context, o RestoreOptions) (Manifest, error) {
	var m Manifest
	if err := privateFile(o.IdentityFile); err != nil {
		return m, err
	}
	raw, err := os.ReadFile(o.IdentityFile)
	if err != nil {
		return m, err
	}
	identities, err := age.ParseIdentities(strings.NewReader(string(raw)))
	if err != nil {
		return m, err
	}
	input, err := os.Open(o.Input)
	if err != nil {
		return m, err
	}
	defer input.Close()
	decrypted, err := age.Decrypt(input, identities...)
	if err != nil {
		return m, err
	}
	if _, err = os.Lstat(o.Destination); !os.IsNotExist(err) {
		return m, errors.New("restore destination must be a new path")
	}
	parent := filepath.Dir(o.Destination)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return m, err
	}
	stage, err := os.MkdirTemp(parent, ".restore-")
	if err != nil {
		return m, err
	}
	defer os.RemoveAll(stage)
	zipped, err := gzip.NewReader(decrypted)
	if err != nil {
		return m, err
	}
	archive := tar.NewReader(zipped)
	if o.MaxBytes <= 0 {
		o.MaxBytes = 64 << 30
	}
	var total int64
	seen := map[string]bool{}
	for {
		if err = ctx.Err(); err != nil {
			return m, err
		}
		h, e := archive.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return m, e
		}
		allowed := h.Name == "manifest.json" || h.Name == "config.json" || h.Name == "database.sql" || h.Name == "database.sqlite" || strings.HasPrefix(h.Name, "releases/") && safeRelease.MatchString(strings.TrimPrefix(h.Name, "releases/"))
		if !allowed || h.Typeflag != tar.TypeReg || seen[h.Name] || h.Size < 0 || h.Size > o.MaxBytes-total || len(seen) > 100000 {
			return m, errors.New("invalid or oversized backup archive")
		}
		if h.Name == "manifest.json" && h.Size > 16<<20 {
			return m, errors.New("manifest too large")
		}
		if h.Name == "config.json" && h.Size > 1<<20 {
			return m, errors.New("config too large")
		}
		seen[h.Name] = true
		total += h.Size
		target := filepath.Join(stage, filepath.FromSlash(h.Name))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return m, err
		}
		f, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return m, e
		}
		_, e = io.Copy(f, archive)
		e = errors.Join(e, f.Close())
		if e != nil {
			return m, e
		}
	}
	// Read to the end to authenticate age's final chunk and gzip's checksum.
	if n, e := io.Copy(io.Discard, io.LimitReader(zipped, 1025)); e != nil || n > 1024 {
		err = errors.New("invalid archive trailing data")
		return m, err
	}
	if err = zipped.Close(); err != nil {
		return m, err
	}
	if n, e := io.Copy(io.Discard, io.LimitReader(decrypted, 1025)); e != nil || n != 0 {
		return m, errors.New("backup trailing data or authentication failed")
	}
	raw, err = os.ReadFile(filepath.Join(stage, "manifest.json"))
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(raw, &m); err != nil {
		return m, err
	}
	if m.Format != 1 || (m.Driver != "sqlite" && m.Driver != "mysql") || len(seen) != len(m.Files)+1 {
		return m, errors.New("unsupported backup manifest")
	}
	for name, want := range m.Files {
		if !seen[name] || name == "manifest.json" {
			return m, errors.New("missing backup file")
		}
		got, e := fileEntry(filepath.Join(stage, name))
		if e != nil || got != want {
			return m, fmt.Errorf("backup checksum mismatch: %s", name)
		}
	}
	var cfg config.DatabaseConfig
	raw, err = os.ReadFile(filepath.Join(stage, "config.json"))
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return m, err
	}
	if cfg.Driver != m.Driver || len(cfg.DataKey) != 64 || len(cfg.CardPepper) < 16 {
		return m, errors.New("backup effective keys/configuration invalid")
	}
	destination, err := filepath.Abs(o.Destination)
	if err != nil {
		return m, err
	}
	if m.Driver == "sqlite" {
		if !seen["database.sqlite"] || seen["database.sql"] {
			return m, errors.New("SQLite snapshot missing")
		}
		cfg.DSN = "file:" + filepath.ToSlash(filepath.Join(destination, "database.sqlite")) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	} else {
		if !seen["database.sql"] || seen["database.sqlite"] {
			return m, errors.New("MySQL snapshot missing")
		}
		if err = privateFile(o.MySQLDSNFile); err != nil {
			return m, err
		}
		raw, err = os.ReadFile(o.MySQLDSNFile)
		if err != nil {
			return m, err
		}
		cfg.DSN = strings.TrimSpace(string(raw))
		st, e := store.Open(ctx, "mysql", cfg.DSN)
		if e != nil {
			return m, e
		}
		empty, e := st.DatabaseEmpty(ctx)
		st.Close()
		if e != nil {
			return m, e
		}
		if !empty {
			return m, errors.New("restore requires a completely empty MySQL database")
		}
		sqlFile, e := os.Open(filepath.Join(stage, "database.sql"))
		if e != nil {
			return m, e
		}
		err = store.MySQLCommand(ctx, cfg.DSN, "mysql", sqlFile, io.Discard)
		sqlFile.Close()
		if err != nil {
			return m, err
		}
	}
	if err = config.SaveDatabaseConfig(filepath.Join(stage, "config.json"), cfg); err != nil {
		return m, err
	}
	// ReleaseDir is deployment configuration; restore writes a private runtime
	// template so effective keys and package location travel together.
	quote := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }
	runtime := fmt.Sprintf("YN_DB_CONFIG_FILE=%s\nYN_RELEASE_DIR=%s\n", quote(filepath.Join(destination, "config.json")), quote(filepath.Join(destination, "releases")))
	if err = os.WriteFile(filepath.Join(stage, "runtime.env"), []byte(runtime), 0600); err != nil {
		return m, err
	}
	if err = os.Rename(stage, destination); err != nil {
		return m, err
	}
	return m, nil
}

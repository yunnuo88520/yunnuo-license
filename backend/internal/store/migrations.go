package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/filelock"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) migrationLock(ctx context.Context) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	if s.driver == "mysql" {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			cancel()
			return nil, err
		}
		var db string
		if err = conn.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&db); err != nil {
			conn.Close()
			cancel()
			return nil, err
		}
		hash := sha256.Sum256([]byte(db))
		name := "yunnuo:migrate:" + hex.EncodeToString(hash[:16])
		var locked int
		if err = conn.QueryRowContext(ctx, `SELECT GET_LOCK(?,25)`, name).Scan(&locked); err != nil || locked != 1 {
			conn.Close()
			cancel()
			if err == nil {
				err = fmt.Errorf("database migration lock timed out")
			}
			return nil, err
		}
		return func() {
			release, done := context.WithTimeout(context.Background(), 3*time.Second)
			defer done()
			conn.ExecContext(release, `SELECT RELEASE_LOCK(?)`, name)
			conn.Close()
			cancel()
		}, nil
	}
	name, _, _ := strings.Cut(strings.TrimPrefix(s.dsn, "file:"), "?")
	if name == ":memory:" || strings.Contains(s.dsn, "mode=memory") {
		cancel()
		return func() {}, nil
	}
	abs, err := filepath.Abs(name)
	if err != nil {
		cancel()
		return nil, err
	}
	unlock, err := filelock.Acquire(ctx, abs+".migrate.lock", false)
	if err != nil {
		cancel()
		return nil, err
	}
	return func() { unlock(); cancel() }, nil
}
func (s *Store) initializeMigrationHistory(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name VARCHAR(255) PRIMARY KEY,sha256 VARCHAR(64) NOT NULL,applied_at VARCHAR(40) NOT NULL)`)
	return err
}
func (s *Store) applyMigration(ctx context.Context, name string, script []byte) error {
	hash := sha256.Sum256(script)
	digest := hex.EncodeToString(hash[:])
	var old string
	err := s.db.QueryRowContext(ctx, `SELECT sha256 FROM schema_migrations WHERE name=?`, name).Scan(&old)
	if err == nil {
		if old != digest {
			return fmt.Errorf("applied migration %s checksum changed; restore the original script and add a new migration", name)
		}
		return nil
	}
	if !IsNotFound(err) {
		return err
	}
	if s.driver == "sqlite" {
		return s.WithTx(ctx, func(tx *Tx) error {
			if _, err := tx.tx.ExecContext(ctx, string(script)); err != nil {
				return err
			}
			_, err := tx.tx.ExecContext(ctx, `INSERT INTO schema_migrations(name,sha256,applied_at) VALUES(?,?,?)`, name, digest, ts(time.Now()))
			return err
		})
	}
	// MySQL DDL commits implicitly. Scripts must be resumable; restore a verified
	// backup for rollback. The advisory lock prevents concurrent startup migrations.
	if _, err = s.db.ExecContext(ctx, string(script)); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO schema_migrations(name,sha256,applied_at) VALUES(?,?,?)`, name, digest, ts(time.Now()))
	return err
}

// LockSchema coordinates a full backup with startup migrations. Business
// transactions do not take this lock and can continue during the snapshot.
func (s *Store) LockSchema(ctx context.Context) (func(), error) { return s.migrationLock(ctx) }

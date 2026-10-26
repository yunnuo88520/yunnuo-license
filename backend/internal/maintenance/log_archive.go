package maintenance

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"filippo.io/age"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/config"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"os"
	"path/filepath"
	"time"
)

func ArchiveLogs(ctx context.Context, cfg config.Config, table, recipient, output string, keepDays, limit int) (int, error) {
	if keepDays < 30 {
		return 0, errors.New("at least 30 days must remain online")
	}
	r, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		return 0, err
	}
	if _, err = os.Lstat(output); !os.IsNotExist(err) {
		return 0, errors.New("archive output must not exist")
	}
	if err = os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return 0, err
	}
	st, err := store.Open(ctx, cfg.DatabaseDriver, cfg.Database)
	if err != nil {
		return 0, err
	}
	defer st.Close()
	before := time.Now().UTC().AddDate(0, 0, -keepDays)
	return st.ArchiveLogBatch(ctx, table, before, limit, func(records []json.RawMessage) error {
		f, err := os.CreateTemp(filepath.Dir(output), ".log-archive-")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		encrypted, err := age.Encrypt(f, r)
		if err != nil {
			f.Close()
			return err
		}
		zip := gzip.NewWriter(encrypted)
		encoder := json.NewEncoder(zip)
		err = encoder.Encode(map[string]any{"format": 1, "table": table, "before": before, "records": len(records), "created_at": time.Now().UTC()})
		for _, row := range records {
			if err != nil {
				break
			}
			err = encoder.Encode(row)
		}
		err = errors.Join(err, zip.Close(), encrypted.Close(), f.Sync(), f.Close())
		if err != nil {
			return err
		}
		return os.Link(f.Name(), output)
	})
}

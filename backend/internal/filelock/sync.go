package filelock

import (
	"errors"
	"os"
)

// SyncDirectory makes a rename/link durable before its related SQL commit.
// Syncing the file alone does not persist its directory entry across power loss.
func SyncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

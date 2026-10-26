package maintenance

import (
	"context"
	"database/sql"
	"filippo.io/age"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/config"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogArchivePreservesRecentRecordsAndNeverDeletesOnFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.Config{DatabaseDriver: "sqlite", Database: "file:" + filepath.Join(dir, "logs.db")}
	st, e := store.Open(ctx, "sqlite", cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	if e = st.Migrate(ctx, "../../migrations"); e != nil {
		t.Fatal(e)
	}
	db, _ := sql.Open("sqlite", cfg.Database)
	defer db.Close()
	for i, at := range []time.Time{time.Now().AddDate(0, 0, -400), time.Now()} {
		_, e = db.Exec(`INSERT INTO operation_logs VALUES(?,?,'test','system','','','','GET','/test','',200,'success','',?)`, string(rune('a'+i)), string(rune('a'+i)), at.UTC().Format(time.RFC3339Nano))
		if e != nil {
			t.Fatal(e)
		}
	}
	key, _ := age.GenerateX25519Identity()
	out := filepath.Join(dir, "archive.age")
	os.WriteFile(out, []byte("exists"), 0600)
	if _, e = ArchiveLogs(ctx, cfg, "operation_logs", key.Recipient().String(), out, 365, 100); e == nil {
		t.Fatal("overwrote archive")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM operation_logs`).Scan(&n)
	if n != 2 {
		t.Fatal("deleted on failure")
	}
	os.Remove(out)
	n, e = ArchiveLogs(ctx, cfg, "operation_logs", key.Recipient().String(), out, 365, 100)
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	db.QueryRow(`SELECT COUNT(*) FROM operation_logs`).Scan(&n)
	if n != 1 {
		t.Fatal("recent logs deleted")
	}
	if _, e = ArchiveLogs(ctx, cfg, "agent_money_ledgers", key.Recipient().String(), filepath.Join(dir, "finance.age"), 365, 100); e == nil {
		t.Fatal("allowed financial archive")
	}
}

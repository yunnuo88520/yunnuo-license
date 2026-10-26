package store

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestVersionedMigrationChecksumsAndSQLiteAtomicFailure(t *testing.T) {
	ctx := context.Background()
	s, e := Open(ctx, "sqlite", "file:"+filepath.Join(t.TempDir(), "db.sqlite")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.initializeMigrationHistory(ctx); e != nil {
		t.Fatal(e)
	}
	script := []byte(`CREATE TABLE test_migration(id INTEGER PRIMARY KEY); INSERT INTO test_migration(id) VALUES(1);`)
	if e = s.applyMigration(ctx, "001-test.sql", script); e != nil {
		t.Fatal(e)
	}
	if e = s.applyMigration(ctx, "001-test.sql", script); e != nil {
		t.Fatal("reapplied non-idempotent script", e)
	}
	if e = s.applyMigration(ctx, "001-test.sql", append(script, ' ')); e == nil {
		t.Fatal("accepted edited historical migration")
	}
	if e = s.applyMigration(ctx, "002-fail.sql", []byte(`CREATE TABLE partial_migration(id INTEGER); INSERT INTO missing_table VALUES(1);`)); e == nil {
		t.Fatal("invalid migration accepted")
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='partial_migration'`).Scan(&count)
	if count != 0 {
		t.Fatal("partial schema committed")
	}
}
func TestConcurrentMigrationsApplyEachVersionOnce(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()
	dir := filepath.Join(folder, "migrations")
	os.Mkdir(dir, 0700)
	entries, _ := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	for _, p := range entries {
		raw, _ := os.ReadFile(p)
		os.WriteFile(filepath.Join(dir, filepath.Base(p)), raw, 0600)
	}
	os.WriteFile(filepath.Join(dir, "999-once.sql"), []byte(`CREATE TABLE migration_counter(id INTEGER); INSERT INTO migration_counter(id) VALUES(1);`), 0600)
	dsn := "file:" + filepath.Join(folder, "db.sqlite") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	stores := []*Store{}
	for n := 0; n < 2; n++ {
		s, e := Open(ctx, "sqlite", dsn)
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		stores = append(stores, s)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for _, s := range stores {
		wg.Add(1)
		go func(s *Store) { defer wg.Done(); errors <- s.Migrate(ctx, dir) }(s)
	}
	wg.Wait()
	close(errors)
	for e := range errors {
		if e != nil {
			t.Fatal(e)
		}
	}
	var count int
	stores[0].db.QueryRow(`SELECT COUNT(*) FROM migration_counter`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate migration", count)
	}
}

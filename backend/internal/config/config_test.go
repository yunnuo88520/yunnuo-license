package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveDatabaseConfigWritesPrivateReloadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "database.json")
	want := DatabaseConfig{Driver: "mysql", DSN: "user:secret@tcp(db:3306)/licenses"}
	if err := SaveDatabaseConfig(path, want); err != nil {
		t.Fatalf("save database config: %v", err)
	}
	got, err := readDatabaseConfig(path)
	if err != nil {
		t.Fatalf("read database config: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected config: got=%#v want=%#v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("database config must not be accessible by group or others: mode=%#o", info.Mode().Perm())
	}
}

func TestLoadPrefersEnvironmentOverSavedDatabaseConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.json")
	if err := SaveDatabaseConfig(path, DatabaseConfig{Driver: "mysql", DSN: "saved-dsn"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YN_DB_CONFIG_FILE", path)
	t.Setenv("YN_DB_DRIVER", "sqlite")
	t.Setenv("YN_DB", "environment-dsn")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseDriver != "sqlite" || cfg.Database != "environment-dsn" {
		t.Fatalf("environment did not override saved config: %#v", cfg)
	}
}

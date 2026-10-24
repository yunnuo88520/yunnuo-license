package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveDatabaseConfigWritesPrivateReloadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "database.json")
	want := DatabaseConfig{Driver: "mysql", DSN: "user:secret@tcp(db:3306)/licenses", AdminPath: "/admin"}
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

func TestLoadUsesInstalledDatabaseOverBootstrapEnvironment(t *testing.T) {
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
	if cfg.DatabaseDriver != "mysql" || cfg.Database != "saved-dsn" {
		t.Fatalf("installed database selection was overridden by bootstrap environment")
	}
}

func TestInstallationKeysAndCustomEntryPersistAcrossRestart(t *testing.T) {
	t.Setenv("YN_DB_CONFIG_FILE", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("YN_DATA_KEY", "")
	t.Setenv("YN_CARD_PEPPER", "")
	a, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err = a.PrepareInstallationKeys(false); err != nil {
		t.Fatal(err)
	}
	f, err := readDatabaseConfig(a.DatabaseConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.DataKey) != 64 || len(f.CardPepper) != 64 {
		t.Fatal("installation did not generate random keys")
	}
	f.AdminPath = "/company/manage"
	if err = SaveDatabaseConfig(a.DatabaseConfigFile, f); err != nil {
		t.Fatal(err)
	}
	b, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if b.AdminPath != "/company/manage" || string(a.CardPepper) != string(b.CardPepper) || string(a.DataKey) != string(b.DataKey) {
		t.Fatal("entry or keys changed on restart")
	}
	for _, path := range []string{"/recover", "/recover/admin", "/install", "/v1/test", "//outside", "/admin/login", "/user-console", "/test?x=1"} {
		if _, err = NormalizeAdminPath(path); err == nil {
			t.Fatalf("unsafe entry accepted: %s", path)
		}
	}
}

func TestLegacyInstallationKeepsItsExistingKeys(t *testing.T) {
	t.Setenv("YN_DB_CONFIG_FILE", filepath.Join(t.TempDir(), "legacy.json"))
	t.Setenv("YN_DATA_KEY", "")
	t.Setenv("YN_CARD_PEPPER", "")
	a, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	oldKey, oldPepper := string(a.DataKey), string(a.CardPepper)
	if err = a.PrepareInstallationKeys(true); err != nil {
		t.Fatal(err)
	}
	b, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(b.DataKey) != oldKey || string(b.CardPepper) != oldPepper {
		t.Fatal("upgrade rotated legacy encryption keys")
	}
}

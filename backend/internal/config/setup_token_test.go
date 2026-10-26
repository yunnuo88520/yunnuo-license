package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupTokenIsPrivatePersistentAndNotRequiredAfterInstall(t *testing.T) {
	t.Setenv("YN_SETUP_TOKEN", "")
	cfg := Config{DatabaseConfigFile: filepath.Join(t.TempDir(), "config.json")}
	token, e := cfg.PrepareSetupToken(false)
	if e != nil || len(token) != 64 {
		t.Fatal(e)
	}
	same, e := cfg.PrepareSetupToken(false)
	if e != nil || same != token {
		t.Fatal("token changed across restart", e)
	}
	info, _ := os.Stat(cfg.SetupTokenPath())
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if installed, e := cfg.PrepareSetupToken(true); e != nil || installed != "" {
		t.Fatal(e)
	}
	t.Setenv("YN_SETUP_TOKEN", "short")
	if _, e = cfg.PrepareSetupToken(false); e == nil {
		t.Fatal("weak explicit token accepted")
	}
}

package maintenance

import (
	"context"
	"filippo.io/age"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/config"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncryptedBackupRestoresCompleteUsableInstance(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.Config{DatabaseDriver: "sqlite", Database: "file:" + filepath.Join(dir, "source.db") + "?_pragma=foreign_keys(1)", CardPepper: []byte(strings.Repeat("p", 32)), DataKey: []byte(strings.Repeat("k", 32)), ReleaseDir: filepath.Join(dir, "packages"), AdminPath: "/private-admin"}
	st, err := store.Open(ctx, cfg.DatabaseDriver, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Migrate(ctx, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	svc := service.New(st, cfg.CardPepper, cfg.DataKey).WithReleaseDir(cfg.ReleaseDir)
	defer svc.Close()
	if _, _, err = svc.EnsureBootstrapAdmin(ctx, "root", "backup-password", "Root"); err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreateProduct(ctx, service.CreateProductInput{Name: "Restored Ebox", BindMode: "domain", LicenseMode: "perpetual", UpdateDurationDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := svc.CreateCardBatch(ctx, service.CreateCardBatchInput{ProductID: p.ID, Name: "restore fixture", Quantity: 1, DurationDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	lic, err := svc.Activate(ctx, service.ActivateInput{AppKey: p.AppKey, CardCode: batch.Codes[0], BindMode: "domain", BindValue: "ebox.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SetProductUpdates(ctx, p.ID, true, "root"); err != nil {
		t.Fatal(err)
	}
	release, err := svc.UploadRelease(ctx, service.UploadReleaseInput{ProductID: p.ID, Version: "1.0.0", Notes: "backup fixture", Filename: "ebox.zip"}, strings.NewReader("restorable-package"), "root")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ChangeReleaseStatus(ctx, p.ID, release.ID, "published", "root"); err != nil {
		t.Fatal(err)
	}
	identity, _ := age.GenerateX25519Identity()
	identityPath := filepath.Join(dir, "identity")
	os.WriteFile(identityPath, []byte(identity.String()), 0600)
	output := filepath.Join(dir, "backup.age")
	m, err := Backup(ctx, cfg, identity.Recipient().String(), output)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 3 {
		t.Fatalf("manifest files %d", len(m.Files))
	}
	encrypted, _ := os.ReadFile(output)
	if strings.Contains(string(encrypted), "backup-password") || strings.Contains(string(encrypted), "restorable-package") {
		t.Fatal("plaintext backup")
	}
	target := filepath.Join(dir, "restored")
	if _, err = Restore(ctx, RestoreOptions{Input: output, IdentityFile: identityPath, Destination: target}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YN_DB_CONFIG_FILE", filepath.Join(target, "config.json"))
	restoredCfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(ctx, restoredCfg.DatabaseDriver, restoredCfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	if err = restored.Migrate(ctx, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	recovered := service.New(restored, restoredCfg.CardPepper, restoredCfg.DataKey).WithReleaseDir(filepath.Join(target, "releases"))
	defer recovered.Close()
	if _, err = recovered.AdminLogin(ctx, service.AdminLoginInput{Username: "root", Password: "backup-password"}); err != nil {
		t.Fatal(err)
	}
	input := service.VerifyInput{AppKey: p.AppKey, LicenseNo: lic.LicenseNo, BindMode: "domain", BindValue: "ebox.example.com"}
	rights, err := recovered.Verify(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !rights.CanUse || !rights.CanUpdate {
		t.Fatal("rights lost")
	}
	got, f, err := recovered.DownloadRelease(ctx, release.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got.SHA256 != release.SHA256 {
		t.Fatal("manifest changed")
	}

	// Wrong keys, truncated authenticated data, limits and populated destinations
	// are rejected before any restored instance can become visible.
	wrong, _ := age.GenerateX25519Identity()
	wrongPath := filepath.Join(dir, "wrong")
	os.WriteFile(wrongPath, []byte(wrong.String()), 0600)
	for name, o := range map[string]RestoreOptions{
		"wrong-key": {Input: output, IdentityFile: wrongPath}, "too-large": {Input: output, IdentityFile: identityPath, MaxBytes: 1}, "existing": {Input: output, IdentityFile: identityPath, Destination: target},
	} {
		if o.Destination == "" {
			o.Destination = filepath.Join(dir, name)
		}
		if _, err = Restore(ctx, o); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	corrupt := filepath.Join(dir, "corrupt.age")
	os.WriteFile(corrupt, encrypted[:len(encrypted)-8], 0600)
	if _, err = Restore(ctx, RestoreOptions{Input: corrupt, IdentityFile: identityPath, Destination: filepath.Join(dir, "corrupt")}); err == nil {
		t.Fatal("accepted truncated backup")
	}
	if info, _ := os.Stat(output); info.Mode().Perm() != 0600 {
		t.Fatal("backup is public")
	}

}

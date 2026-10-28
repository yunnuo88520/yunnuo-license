package maintenance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/config"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func TestV2BackupRestoresApplicationKeysHashOnlyCardsAuditAndPackages(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.Config{DatabaseDriver: "sqlite", Database: "file:" + filepath.Join(dir, "source.sqlite") + "?_pragma=foreign_keys(1)", CardPepper: []byte(strings.Repeat("p", 32)), DataKey: []byte(strings.Repeat("k", 32)), ReleaseDir: filepath.Join(dir, "packages")}
	st, e := store.Open(ctx, cfg.DatabaseDriver, cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	if e = st.Migrate(ctx, "../../migrations"); e != nil {
		t.Fatal(e)
	}
	svc := service.New(st, cfg.CardPepper, cfg.DataKey).WithReleaseDir(cfg.ReleaseDir)
	defer svc.Close()
	if _, _, e = svc.EnsureBootstrapAdmin(ctx, "root", "backup-password", "Root"); e != nil {
		t.Fatal(e)
	}
	root := core.BindPrincipal(ctx, core.Principal{Actor: core.Actor{Kind: "admin", ID: "root", Role: "super_admin"}})
	s := svc.Licensing()
	enabled := true
	app, e := s.CreateApplication(root, core.CreateApplicationInput{Name: "Backup product", AllowedModels: []string{"perpetual"}, AllowedBindings: []string{"installation"}, UpdatesEnabled: &enabled})
	if e != nil {
		t.Fatal(e)
	}
	plan, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Standard", Terms: core.Terms{Model: "perpetual", BindingTypes: []string{"installation"}, UpdateDurationDays: 30}})
	if e != nil {
		t.Fatal(e)
	}
	sale, e := s.CreateSale(root, app.AppID, core.SaleInput{PlanID: plan.ID, Quantity: 1, DistributeCards: true, Reason: "verified receipt"}, true)
	if e != nil {
		t.Fatal(e)
	}
	release, e := s.UploadRelease(root, app.AppID, core.UploadInput{Version: "2.0.0", Filename: "product.zip"}, strings.NewReader("restorable v2 package"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SetReleaseStatus(root, app.AppID, release.ID, "published", "release acceptance"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RotateSigningKey(root, app.AppID, "backup with rotation overlap", false); e != nil {
		t.Fatal(e)
	}
	identity, _ := age.GenerateX25519Identity()
	identityPath := filepath.Join(dir, "identity")
	if e = os.WriteFile(identityPath, []byte(identity.String()), 0600); e != nil {
		t.Fatal(e)
	}
	archive := filepath.Join(dir, "v2-backup.age")
	if _, e = Backup(ctx, cfg, identity.Recipient().String(), archive); e != nil {
		t.Fatal(e)
	}
	destination := filepath.Join(dir, "restored")
	if _, e = Restore(ctx, RestoreOptions{Input: archive, IdentityFile: identityPath, Destination: destination}); e != nil {
		t.Fatal(e)
	}
	t.Setenv("YN_DB_CONFIG_FILE", filepath.Join(destination, "config.json"))
	restoredCfg, e := config.Load()
	if e != nil {
		t.Fatal(e)
	}
	restored, e := store.Open(ctx, restoredCfg.DatabaseDriver, restoredCfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	if e = restored.Migrate(ctx, "../../migrations"); e != nil {
		t.Fatal(e)
	}
	recovered := core.New(restored.LicensingRegistry(), restoredCfg.CardPepper, restoredCfg.DataKey).WithReleaseStorage(filepath.Join(destination, "releases"), core.StorageLimits{})
	if _, e = recovered.AuthenticateAppSecret(ctx, app.AppID, app.AppSecret); e != nil {
		t.Fatal("hash-only application credential not restored", e)
	}
	keys, e := recovered.PublicKeys(ctx, app.AppID)
	if e != nil || len(keys.Keys) != 2 {
		t.Fatal("rotation overlap lost", e)
	}
	pub, private, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	proof := func(path string, input any) core.SignedRequest {
		body, _ := json.Marshal(input)
		nonce := make([]byte, 24)
		rand.Read(nonce)
		p := core.RequestProof{Timestamp: time.Now().Unix(), Nonce: base64.RawURLEncoding.EncodeToString(nonce)}
		p.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(core.RequestSigningMessage("POST", path, app.AppID, p.Timestamp, p.Nonce, body))))
		return core.SignedRequest{Method: "POST", Path: path, RemoteIP: "127.0.0.1", Body: body, Proof: p}
	}
	in := core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, PublicKey: base64.RawURLEncoding.EncodeToString(pub), Binding: core.BindingInput{Type: "installation", Value: "persistent-product"}, Form: "gui"}
	activated, e := recovered.Activate(ctx, in, proof("/v2/activate", in))
	if e != nil {
		t.Fatal("restored hash-only card or private signing key failed", e)
	}
	update := core.UpdateInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: in.Binding, Form: "gui", CurrentVersion: "1.0.0", Target: core.ReleaseTarget{OS: "any", Arch: "any", Runtime: "any", Channel: "stable", Kind: "archive"}}
	_, file, e := recovered.DownloadUpdate(ctx, update, release.ID, proof("/v2/updates/"+release.ID+"/download", update))
	if e != nil {
		t.Fatal(e)
	}
	raw, e := io.ReadAll(file)
	file.Close()
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != release.SHA256 {
		t.Fatal("restored package content changed")
	}
	if _, e = recovered.AuditIntegrity(root, app.AppID); e != nil {
		t.Fatal("restored audit chain invalid", e)
	}
}

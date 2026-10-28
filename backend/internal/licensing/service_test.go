package licensing_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mysqlDriver "github.com/go-sql-driver/mysql"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func coreFixture(t *testing.T) (*core.Service, core.Registry, context.Context, *time.Time) {
	t.Helper()
	driver, dsn := "sqlite", "file:"+filepath.Join(t.TempDir(), "core.sqlite")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	if configured := os.Getenv("YN_V2_MYSQL_DSN"); configured != "" {
		cfg, e := mysqlDriver.ParseDSN(configured)
		if e != nil {
			t.Fatal(e)
		}
		cfg.DBName = ""
		cfg.MultiStatements = true
		cfg.ParseTime = false
		root, e := sql.Open("mysql", cfg.FormatDSN())
		if e != nil {
			t.Fatal(e)
		}
		var suffix [8]byte
		if _, e = rand.Read(suffix[:]); e != nil {
			t.Fatal(e)
		}
		name := "v2_test_" + hex.EncodeToString(suffix[:])
		if _, e = root.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); e != nil {
			root.Close()
			t.Fatal(e)
		}
		t.Cleanup(func() {
			if _, e := root.Exec("DROP DATABASE `" + name + "`"); e != nil {
				t.Error(fmt.Errorf("drop isolated MySQL test database: %w", e))
			}
			root.Close()
		})
		cfg.DBName = name
		driver, dsn = "mysql", cfg.FormatDSN()
	}
	st, e := store.Open(context.Background(), driver, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	if e = st.Migrate(context.Background(), "../../migrations"); e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	registry := st.LicensingRegistry()
	service := core.New(registry, []byte("test-pepper"), []byte("0123456789abcdef0123456789abcdef")).WithClock(func() time.Time { return now })
	ctx := core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "admin", ID: "root", Role: "super_admin"}})
	return service, registry, ctx, &now
}
func TestApplicationSecretsAndRotationStayWithinTheirScope(t *testing.T) {
	service, registry, root, now := coreFixture(t)
	a, e := service.CreateApplication(root, core.CreateApplicationInput{Name: "Ebox"})
	if e != nil {
		t.Fatal(e)
	}
	b, e := service.CreateApplication(root, core.CreateApplicationInput{Name: "Other"})
	if e != nil {
		t.Fatal(e)
	}
	if a.AppID == b.AppID || len(a.AppID) != 18 || len(a.AppSecret) != 32 {
		t.Fatal("bad application identity")
	}
	if _, e = service.AuthenticateAppSecret(context.Background(), a.AppID, a.AppSecret); e != nil {
		t.Fatal(e)
	}
	if _, e = service.AuthenticateAppSecret(context.Background(), b.AppID, a.AppSecret); e == nil {
		t.Fatal("cross-application secret accepted")
	}
	stored, e := registry.GetApplication(root, a.AppID)
	if e != nil || stored.SecretHash == "" || strings.Contains(stored.SecretHash, a.AppSecret) {
		t.Fatal("secret not hashed", e)
	}
	encoded, _ := json.Marshal(stored)
	if strings.Contains(string(encoded), "secret_hash") || strings.Contains(string(encoded), a.AppSecret) {
		t.Fatal("secret leaked by application serialization")
	}
	keys, e := service.PublicKeys(context.Background(), a.AppID)
	if e != nil || len(keys.Keys) != 1 {
		t.Fatal(keys, e)
	}
	old := keys.Keys[0].KID
	if _, e = service.RotateSigningKey(root, a.AppID, "scheduled rotation", false); e != nil {
		t.Fatal(e)
	}
	keys, e = service.PublicKeys(context.Background(), a.AppID)
	if e != nil || len(keys.Keys) != 2 {
		t.Fatal("overlap missing", keys, e)
	}
	other, e := service.PublicKeys(context.Background(), b.AppID)
	if e != nil || len(other.Keys) != 1 || other.Keys[0].KID == old {
		t.Fatal("another application was affected")
	}
	*now = now.Add(9 * 24 * time.Hour)
	keys, e = service.PublicKeys(context.Background(), a.AppID)
	if e != nil || len(keys.Keys) != 1 || keys.Keys[0].KID == old {
		t.Fatal("retired key did not expire", keys, e)
	}
	reset, e := service.ResetAppSecret(root, a.AppID, "credential lifecycle test")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.AuthenticateAppSecret(context.Background(), a.AppID, a.AppSecret); e == nil {
		t.Fatal("old secret survived reset")
	}
	if _, e = service.AuthenticateAppSecret(context.Background(), a.AppID, reset["app_secret"].(string)); e != nil {
		t.Fatal(e)
	}
}
func TestDeveloperCannotManageAnotherApplication(t *testing.T) {
	service, _, root, _ := coreFixture(t)
	a, e := service.CreateApplication(root, core.CreateApplicationInput{Name: "A", DeveloperID: "developer-A"})
	if e != nil {
		t.Fatal(e)
	}
	developer := core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "developer", ID: "developer-B", Role: "developer"}})
	if _, e = service.GetApplication(developer, a.AppID); e == nil {
		t.Fatal("cross-developer application exposed")
	}
	if _, e = service.ResetAppSecret(developer, a.AppID, "not mine"); e == nil {
		t.Fatal("cross-developer reset allowed")
	}
	own, e := service.CreateApplication(developer, core.CreateApplicationInput{Name: "B", DeveloperID: "developer-A"})
	if e != nil || own.Application.DeveloperID != "developer-B" {
		t.Fatal("developer spoofed application ownership", own, e)
	}
	apps, e := service.ListApplications(developer)
	if e != nil || len(apps) != 1 || apps[0].ApplicationID != own.AppID {
		t.Fatal(apps, e)
	}
}

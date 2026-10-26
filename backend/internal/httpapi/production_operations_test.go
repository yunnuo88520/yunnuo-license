package httpapi

import (
	"context"
	"database/sql"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupOwnershipTokenAndSingleUseInitialization(t *testing.T) {
	s := newHTTPTestService(t)
	token := strings.Repeat("a", 64)
	dir := t.TempDir()
	h := New(s, "", "", "").WithSetupConfig(filepath.Join("..", "..", "migrations"), filepath.Join(dir, "config.json")).WithSetupToken(token).Handler()
	body := `{"database_driver":"sqlite","database_dsn":"file:` + filepath.ToSlash(filepath.Join(dir, "new.sqlite")) + `?_pragma=foreign_keys(1)","admin_username":"root","admin_password":"secret123"}`
	performJSONStatus(t, h, "POST", "/v1/setup/initialize", "", map[string]any{}, 403)
	call := func(provided string) int {
		r := httptest.NewRequest("POST", "/v1/setup/initialize", strings.NewReader(body))
		r.Header.Set("X-Setup-Token", provided)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if code := call("wrong"); code != 403 {
		t.Fatal(code)
	}
	if code := call(token); code != 200 {
		t.Fatal(code)
	}
	if code := call(token); code != 403 {
		t.Fatal("reinstalled", code)
	}
}
func TestReadinessDetectsFailedAuditStorageWhileLivenessStaysUp(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "readiness.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	st, e := store.Open(ctx, "sqlite", dsn)
	if e != nil {
		t.Fatal(e)
	}
	if e = st.Migrate(ctx, filepath.Join("..", "..", "migrations")); e != nil {
		t.Fatal(e)
	}
	s := service.New(st, []byte("pepper"), []byte("0123456789abcdef0123456789abcdef")).WithReleaseDir(t.TempDir())
	defer s.Close()
	s.EnsureBootstrapAdmin(ctx, "root", "secret123", "Root")
	h := New(s, "", "", "").WithMetricsToken("test-metrics-token").Handler()
	request := func(path, token string) (int, string) {
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	if code, _ := request("/readyz", ""); code != 200 {
		t.Fatal(code)
	}
	if code, _ := request("/metrics", ""); code != 403 {
		t.Fatal("metrics leaked", code)
	}
	raw, _ := sql.Open("sqlite", dsn)
	defer raw.Close()
	raw.Exec(`CREATE TRIGGER fail_probe BEFORE INSERT ON operation_logs BEGIN SELECT RAISE(ABORT,'simulated write failure'); END`)
	if code, _ := request("/readyz", ""); code != 503 {
		t.Fatal("unusable database looked ready", code)
	}
	if code, _ := request("/healthz", ""); code != 200 {
		t.Fatal("liveness confused with readiness", code)
	}
	if code, body := request("/metrics", "test-metrics-token"); code != 200 || !strings.Contains(body, "yunnuo_ready 0") {
		t.Fatal(code, body)
	}
}

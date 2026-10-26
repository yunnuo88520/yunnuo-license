package httpapi

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNginxPageGatePreservesInstallationAndCustomAdminEntry(t *testing.T) {
	s := newHTTPTestService(t)
	api := New(s, "", "", "").WithAdminPath("/private/control").WithFrontendServing(false)
	h := api.Handler()
	gate := func(path string) (int, string) {
		r := httptest.NewRequest("GET", "/v1/setup/page", nil)
		r.Header.Set("X-Yunnuo-Request-URI", path)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Header().Get("X-Yunnuo-Frontend-File")
	}
	for _, path := range []string{"/", "/private/control", "/agent-console/", "/recover"} {
		if code, file := gate(path); code != 401 || file != "" {
			t.Fatal("installation bypass", path, code, file)
		}
	}
	if code, file := gate("/install?from=home"); code != 204 || file != "install/index.html" {
		t.Fatal(code, file)
	}
	if _, _, err := s.EnsureBootstrapAdmin(context.Background(), "root", "test-password", "Root"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"/": "index.html", "/private/control/": "admin-console/index.html", "/agent-console/": "agent-console/index.html", "/user-console": "user-console/index.html", "/developer-console/": "developer-console/index.html", "/recover": "recover/index.html"} {
		if code, file := gate(path); code != 204 || file != want {
			t.Fatal(path, code, file)
		}
	}
	for _, path := range []string{"/admin", "/admin-console/index.html", "/config/runtime.env", "/../backend/yn-license-config.json", "//private/control"} {
		if code, file := gate(path); code != 403 || file != "" {
			t.Fatal("unexpected page", path, code, file)
		}
	}
}
func TestAPIOnlyModeDoesNotServeHTMLOrAssets(t *testing.T) {
	s := newHTTPTestService(t)
	s.EnsureBootstrapAdmin(context.Background(), "root", "test-password", "Root")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("not served by Go"), 0644)
	h := New(s, dir, dir, dir).WithFrontendServing(false).Handler()
	for _, path := range []string{"/", "/admin", "/install", "/assets/anything.js", "/agent-console/", "/user-console/", "/developer-console/", "/recover"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatal(path, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/v1/setup/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}

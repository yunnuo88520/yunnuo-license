package httpapi

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallGateAndConfiguredManagementEntry(t *testing.T) {
	s := newHTTPTestService(t)
	root := t.TempDir()
	for name, body := range map[string]string{"index.html": "public-home", "install/index.html": "install-page", "admin-console/index.html": "management-page"} {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0700)
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	api := New(s, root, filepath.Join(root, "admin-console"), "").WithAdminPath("/private-panel")
	h := api.Handler()
	for _, path := range []string{"/", "/admin", "/private-panel", "/private-panel/", "/agent-console/", "/user-console/", "/developer-console/"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 307 || r.Header().Get("Location") != "/install" {
			t.Fatalf("uninitialized page %s: %d", path, r.Code)
		}
	}
	performJSONStatus(t, h, "POST", "/admin/login", "", map[string]string{"username": "root", "password": "secret123"}, 503)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/install", nil))
	if r.Code != 200 || !strings.Contains(r.Body.String(), "install-page") {
		t.Fatal("installer unavailable")
	}
	if _, _, err := s.EnsureBootstrapAdmin(context.Background(), "root", "secret123", "Root"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]int{"/private-panel": 200, "/private-panel/": 200, "/admin": 404, "/admin/": 404, "/admin-console/": 404} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != want {
			t.Fatalf("entry %s: %d", path, r.Code)
		}
		if want == 200 && !strings.Contains(r.Body.String(), "management-page") {
			t.Fatal("wrong entry content")
		}
	}
	performJSONStatus(t, h, "POST", "/v1/setup/initialize", "", map[string]string{}, 403)
}

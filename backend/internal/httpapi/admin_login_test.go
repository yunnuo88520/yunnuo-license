package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminLoginStepsShareTheExistingFailureLock(t *testing.T) {
	s := newHTTPTestService(t)
	if _, _, err := s.EnsureBootstrapAdmin(context.Background(), "root", "secret123", "Root"); err != nil {
		t.Fatal(err)
	}
	api := New(s, "", "", "")
	api.adminAuthFailLock = newFailureLockout(2, time.Minute)
	h := api.Handler()
	call := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("/admin/login/start", `{"username":"root","password":"secret123"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"login":{"access_token"`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("password-only login failed or was cacheable", w.Code, w.Body.String())
	}
	if w = call("/admin/login/start", `{"username":"root","password":"wrong"}`); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w = call("/admin/login/mfa", `{"challenge":"invalid","code":"123456"}`); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/admin/login", "/admin/login/start", "/admin/login/mfa"} {
		if w = call(path, `{}`); w.Code != 429 || w.Header().Get("Retry-After") == "" {
			t.Fatal("login routes did not share the failure lock", path, w.Code)
		}
	}
}

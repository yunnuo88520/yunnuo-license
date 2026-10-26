package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestTrustedProxyChainIgnoresUntrustedHeadersAndInjectedLeftHop(t *testing.T) {
	api := New(nil, "", "", "").WithTrustedProxyHeaders(true).WithTrustedProxyNetworks([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	for _, tc := range []struct{ peer, header, want string }{
		{"192.0.2.10:123", "203.0.113.4", "192.0.2.10"},
		{"10.0.0.1:123", "203.0.113.4", "203.0.113.4"},
		{"10.0.0.1:123", "198.51.100.99, 203.0.113.4", "203.0.113.4"},
		{"10.0.0.1:123", "203.0.113.4, 10.0.0.2", "203.0.113.4"},
		{"10.0.0.1:123", "bad, 203.0.113.4", "10.0.0.1"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-For", tc.header)
		if got := api.clientIP(r); got != tc.want {
			t.Fatalf("%+v got %s", tc, got)
		}
	}
}
func TestForwardedClientsHaveIndependentLimitsAndFailureLockKeys(t *testing.T) {
	s := newHTTPTestService(t)
	s.EnsureBootstrapAdmin(context.Background(), "root", "secret123", "Root")
	api := New(s, "", "", "").WithTrustedProxyHeaders(true)
	api.verifyFailLock = newFailureLockout(2, time.Minute)
	h := api.Handler()
	call := func(path, ip string) int {
		body := `{"username":"absent","password":"not-correct"}`
		if strings.Contains(path, "verify") {
			body = `{"app_key":"invalid","license_no":"invalid","bind_mode":"device","bind_value":"x"}`
		}
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", ip)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for n := 1; n <= 25; n++ {
		if code := call("/admin/login", fmt.Sprintf("198.51.100.%d", n)); code != 401 {
			t.Fatal("different clients share a limit", n, code)
		}
	}
	for n := 0; n < 10; n++ {
		code := call("/admin/login", "203.0.113.1")
		if code != 401 {
			t.Fatal(code)
		}
	}
	if code := call("/admin/login", "203.0.113.1"); code != 429 {
		t.Fatal(code)
	}
	for n := 0; n < 2; n++ {
		if code := call("/v1/licenses/verify", "203.0.113.2"); code == 429 {
			t.Fatal("premature lock")
		}
	}
	if code := call("/v1/licenses/verify", "203.0.113.2"); code != 429 {
		t.Fatal("lockout checks a different key", code)
	}
	if code := call("/v1/licenses/verify", "203.0.113.3"); code == 429 {
		t.Fatal("another client was locked")
	}
}
func TestAdminNetworkAndJSONLimitsAndSecurityHeaders(t *testing.T) {
	s := newHTTPTestService(t)
	s.EnsureBootstrapAdmin(context.Background(), "root", "secret123", "Root")
	api := New(s, "", "", "").WithAdminNetworks([]netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")})
	h := api.Handler()
	r := httptest.NewRequest("POST", "/admin/login", strings.NewReader(`{}`))
	r.RemoteAddr = "203.0.113.1:123"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("POST", "/admin/login", strings.NewReader(`{"username":"`+strings.Repeat("x", (1<<20)+1)+`","password":"x"}`))
	r.RemoteAddr = "192.0.2.1:123"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, header := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy", "Cache-Control"} {
		if w.Header().Get(header) == "" {
			t.Fatal("missing", header)
		}
	}
}

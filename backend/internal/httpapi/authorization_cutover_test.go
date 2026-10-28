package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestProductionAuthorizationCutover(t *testing.T) {
	api := &API{authorizationV2Only: true}
	called := false
	h := api.authorizationCutover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }))
	for _, route := range []struct{ method, path string }{
		{"POST", "/v1/licenses/activate"}, {"POST", "/v1/app/licenses/verify"}, {"POST", "/v1/licenses/../licenses/verify"},
		{"POST", "/v1/updates/r/download"}, {"GET", "/v1/updates/r/download"}, {"POST", "/developer/products/p/card-batches"},
		{"GET", "/admin/products/p/credentials"}, {"GET", "/admin/card-batches/b/export"},
		{"POST", "/admin/packages"}, {"POST", "/admin/licenses/restore"}, {"POST", "/user/licenses/l/renew"}, {"POST", "/agent/card-batches"},
	} {
		called = false
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(route.method, route.path, strings.NewReader("{}")))
		if called || w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "AUTHORIZATION_V2_REQUIRED") {
			t.Fatalf("retired route reachable: %s %s: %d", route.method, route.path, w.Code)
		}
	}
	for _, route := range []struct{ method, path string }{{"POST", "/v2/activate"}, {"POST", "/v2/verify"}, {"POST", "/v1/setup/initialize"}, {"GET", "/v1/captcha"}, {"POST", "/admin/agent-tiers"}, {"POST", "/admin/agents/a/balance"}, {"POST", "/admin/password"}, {"GET", "/admin/products"}} {
		called = false
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(route.method, route.path, nil))
		if !called || w.Code != 204 {
			t.Fatalf("current route blocked: %s", route.path)
		}
	}
}

func TestV2AdministrativeRoutesEnforceNetworkPolicy(t *testing.T) {
	s := newHTTPTestService(t)
	if _, _, e := s.EnsureBootstrapAdmin(context.Background(), "root", "secret123", "Root"); e != nil {
		t.Fatal(e)
	}
	api := New(s, "", "", "").WithAdminNetworks([]netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}).WithAuthorizationV2Only()
	h := api.Handler()
	token := loginAdmin(t, h, "root", "secret123")
	for _, route := range []string{"/v2/console/applications", "/v2/platform/authorization-archive", "/v2/platform/developers"} {
		for _, entry := range []struct {
			ip     string
			status int
		}{{"192.0.2.2:123", 200}, {"203.0.113.4:123", 403}} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", route, nil)
			r.RemoteAddr = entry.ip
			r.Header.Set("Authorization", "Bearer "+token)
			h.ServeHTTP(w, r)
			if w.Code != entry.status {
				t.Fatalf("%s from %s: %d %s", route, entry.ip, w.Code, w.Body.String())
			}
		}
	}
}

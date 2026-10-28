package httpapi

import (
	"net/http"
	"path"
	"strings"

	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
)

// WithAuthorizationV2Only is set by the production entrypoint. Legacy handlers
// remain available to regression fixtures, never as a production fallback.
func (api *API) WithAuthorizationV2Only() *API {
	api.authorizationV2Only = true
	return api
}

func retiredAuthorizationRoute(method, rawPath string) bool {
	p := path.Clean(rawPath)
	for _, prefix := range []string{"/v1/licenses", "/v1/cards", "/v1/app", "/v1/updates", "/v1/products", "/v1/security"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	if p == "/user/activate" || strings.HasPrefix(p, "/user/licenses") {
		return true
	}
	// Metadata lists are retained only for the read-only historical archive.
	if strings.HasPrefix(p, "/admin/card-batches/") || strings.HasPrefix(p, "/agent/card-batches/") {
		return true
	}
	for _, prefix := range []string{"/admin/products", "/developer/products", "/admin/packages", "/admin/product-groups", "/admin/agent-policies", "/admin/agent-quotas", "/admin/card-batches", "/admin/cards", "/admin/licenses", "/admin/offline-licenses", "/agent/card-batches"} {
		if (p == prefix || strings.HasPrefix(p, prefix+"/")) && method != http.MethodGet && method != http.MethodHead {
			return true
		}
	}
	// Old credentials, integration kits, update downloads and offline exports
	// must not advertise the retired protocol or expose recoverable card data.
	for _, prefix := range []string{"/admin/products/", "/developer/products/", "/admin/offline-licenses/"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	if strings.HasPrefix(p, "/admin/agents/") && strings.HasSuffix(p, "/packages") {
		return true
	}
	if strings.HasPrefix(p, "/admin/agent-tiers/") && strings.HasSuffix(p, "/prices") {
		return true
	}
	return false
}
func (api *API) authorizationCutover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if api.authorizationV2Only && retiredAuthorizationRoute(r.Method, r.URL.Path) {
			w.Header().Set("Cache-Control", "no-store")
			writeError(w, r, core.Error{Code: "AUTHORIZATION_V2_REQUIRED", Message: "旧授权接口已停用，请从应用授权中心生成新版接入代码；历史记录只读保留", Status: http.StatusGone})
			return
		}
		next.ServeHTTP(w, r)
	})
}

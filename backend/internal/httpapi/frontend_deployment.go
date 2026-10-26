package httpapi

import (
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// API-only deployments keep static HTML and assets entirely on Nginx. Nginx
// asks this small gate which public entry file to serve, so first installation
// and admin_path changes remain effective without regenerating Nginx rules.
func (api *API) WithFrontendServing(enabled bool) *API { api.serveFrontend = enabled; return api }
func (api *API) serveFrontendApp(w http.ResponseWriter, r *http.Request, dir, path string) {
	if !api.serveFrontend {
		http.NotFound(w, r)
		return
	}
	serveStaticApp(w, r, dir, path)
}
func (api *API) frontendPageGate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	uri, err := url.ParseRequestURI(r.Header.Get("X-Yunnuo-Request-URI"))
	if err != nil || !strings.HasPrefix(uri.Path, "/") {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	path := strings.TrimSuffix(uri.Path, "/")
	if path == "" {
		path = "/"
	}
	entries := map[string]string{"/": "index.html", "/install": "install/index.html", "/recover": "recover/index.html", "/agent-console": "agent-console/index.html", "/user-console": "user-console/index.html", "/developer-console": "developer-console/index.html", api.adminPath: "admin-console/index.html"}
	entry, ok := entries[path]
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if path == api.adminPath && len(api.adminNetworks) > 0 {
		ip, e := netip.ParseAddr(api.clientIP(r))
		if e != nil || !containsIP(api.adminNetworks, ip) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
	if path != "/install" && !api.installReady.Load() {
		status, e := api.service.SetupStatus(r.Context())
		if e != nil {
			writeError(w, r, e)
			return
		}
		if !status.Initialized {
			w.Header().Set("WWW-Authenticate", `YunnuoInstall realm="installation"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		api.installReady.Store(true)
	}
	w.Header().Set("X-Yunnuo-Frontend-File", entry)
	w.WriteHeader(http.StatusNoContent)
}

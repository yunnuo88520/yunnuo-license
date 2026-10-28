package httpapi

import (
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"net/http"
	"path/filepath"
	"strings"
)

func (api *API) WithAdminPath(path string) *API { api.adminPath = path; return api }
func (api *API) entries() map[string]string {
	return map[string]string{"admin": api.adminPath, "agent": "/agent-console/", "public": "/", "user": "/user-console/", "developer": "/developer-console/"}
}
func (api *API) installApp(w http.ResponseWriter, r *http.Request) {
	api.serveFrontendApp(w, r, filepath.Join(api.publicStaticDir, "install"), "/")
}
func (api *API) installationGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if api.installReady.Load() || path == "/healthz" || path == "/readyz" || path == "/metrics" || path == "/install" || path == "/install/" || path == "/v1/setup/page" || path == "/v1/setup/status" || path == "/v1/setup/initialize" || strings.HasPrefix(path, "/assets/") {
			next.ServeHTTP(w, r)
			return
		}
		status, err := api.service.SetupStatus(r.Context())
		if err != nil {
			writeError(w, r, err)
			return
		}
		if status.Initialized {
			api.installReady.Store(true)
			next.ServeHTTP(w, r)
			return
		}
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && (path == api.adminPath+"/" || strings.Contains(r.Header.Get("Accept"), "text/html")) {
			http.Redirect(w, r, "/install", http.StatusTemporaryRedirect)
			return
		}
		for _, prefix := range []string{"/admin/", "/agent/", "/user/", "/developer/", "/v1/", "/v2/", "/.well-known/"} {
			if strings.HasPrefix(path, prefix) {
				writeError(w, r, service.AppError{Code: "INSTALL_REQUIRED", Message: "complete installation at /install", HTTPStatus: 503})
				return
			}
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			http.Redirect(w, r, "/install", http.StatusTemporaryRedirect)
			return
		}
		writeError(w, r, service.AppError{Code: "INSTALL_REQUIRED", Message: "complete installation at /install", HTTPStatus: 503})
	})
}

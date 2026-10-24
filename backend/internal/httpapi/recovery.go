package httpapi

import (
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"net/http"
	"path/filepath"
)

func (api *API) registerRecoveryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /recover", func(w http.ResponseWriter, r *http.Request) {
		serveStaticApp(w, r, filepath.Join(api.publicStaticDir, "recover"), "/")
	})
	mux.HandleFunc("GET /admin/recovery/accounts", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.RecoveryAccounts(r.Context(), r.URL.Query().Get("kind"), r.URL.Query().Get("agent_id"), adminSessionFrom(r.Context()).Role)
		commerceResponse(w, r, v, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /admin/recovery/{kind}/{userID}", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in service.RecoveryInput
		if !decode(w, r, &in) {
			return
		}
		a := adminSessionFrom(r.Context())
		v, e := api.service.CreateRecovery(r.Context(), r.PathValue("kind"), r.PathValue("userID"), a.UserID, a.Role, in)
		commerceResponse(w, r, v, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /admin/recovery/{kind}/{userID}/password", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Password string `json:"password"`
			Reason   string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		a := adminSessionFrom(r.Context())
		e := api.service.ResetAccountPassword(r.Context(), r.PathValue("kind"), r.PathValue("userID"), in.Password, in.Reason, a.UserID, a.Role)
		commerceResponse(w, r, map[string]bool{"updated": e == nil}, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /v1/accounts/recovery", withRateLimit(api.adminLoginLimit, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Code     string `json:"code"`
			Password string `json:"password"`
		}
		if !decode(w, r, &in) {
			return
		}
		e := api.service.RedeemRecovery(r.Context(), in.Code, in.Password)
		commerceResponse(w, r, map[string]bool{"updated": e == nil}, e)
	}))
}

package httpapi

import (
	"errors"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"net/http"
)

func (api *API) registerAdminMFA(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/mfa", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		out, err := api.service.MFAStatus(r.Context(), adminSessionFrom(r.Context()).UserID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, out)
	}, adminReadRoles...))
	for _, action := range []string{"begin", "confirm", "disable"} {
		mux.HandleFunc("POST /admin/mfa/"+action, api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Password string `json:"password"`
				Code     string `json:"code"`
			}
			if !decode(w, r, &input) {
				return
			}
			id := adminSessionFrom(r.Context()).UserID
			// Password + OTP attempts share the login failure lock and trusted client IP.
			if ok, _ := api.adminAuthFailLock.Allow(remoteClientIP(r)); !ok {
				writeError(w, r, service.ErrRateLimited)
				return
			}
			var out any
			var err error
			switch action {
			case "begin":
				out, err = api.service.BeginAdminMFA(r.Context(), id, input.Password)
			case "confirm":
				var codes []string
				codes, err = api.service.ConfirmAdminMFA(r.Context(), id, input.Password, input.Code)
				out = map[string]any{"recovery_codes": codes}
			case "disable":
				err = api.service.DisableAdminMFA(r.Context(), id, input.Password, input.Code)
				out = map[string]bool{"disabled": true}
			}
			if err != nil {
				var app service.AppError
				if errors.As(err, &app) && (app.Code == "INVALID_CREDENTIALS" || app.Code == "MFA_REQUIRED") {
					api.adminAuthFailLock.Fail(remoteClientIP(r))
				}
				writeError(w, r, err)
				return
			}
			privateCredentialResponse(w)
			writeOK(w, r, out)
		}, adminReadRoles...))
	}
}

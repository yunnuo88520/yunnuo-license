package httpapi

import (
	"net/http"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func (api *API) registerAdminLoginSteps(mux *http.ServeMux) {
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return withClientFailureLock(api.adminAuthFailLock, withRateLimit(api.adminLoginLimit, next))
	}
	mux.HandleFunc("POST /admin/login/start", guard(func(w http.ResponseWriter, r *http.Request) {
		var input service.AdminLoginInput
		if !decode(w, r, &input) {
			return
		}
		out, err := api.service.StartAdminLogin(r.Context(), input, remoteClientIP(r))
		if err != nil {
			recordClientFailure(api.adminAuthFailLock, remoteClientIP(r), err)
			writeError(w, r, err)
			return
		}
		if out.Login != nil {
			setAdminLoginActor(r, *out.Login)
		}
		privateCredentialResponse(w)
		writeOK(w, r, out)
	}))
	mux.HandleFunc("POST /admin/login/mfa", guard(func(w http.ResponseWriter, r *http.Request) {
		var input service.CompleteAdminLoginInput
		if !decode(w, r, &input) {
			return
		}
		out, err := api.service.CompleteAdminLogin(r.Context(), input, remoteClientIP(r))
		if err != nil {
			recordClientFailure(api.adminAuthFailLock, remoteClientIP(r), err)
			writeError(w, r, err)
			return
		}
		setAdminLoginActor(r, out)
		privateCredentialResponse(w)
		writeOK(w, r, out)
	}))
}

func setAdminLoginActor(r *http.Request, result service.AdminLoginResponse) {
	if actor := service.ActorFrom(r.Context()); actor != nil {
		actor.Type, actor.ID, actor.Role = "admin", result.User.ID, result.User.Role
	}
}

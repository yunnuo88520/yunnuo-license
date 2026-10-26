package httpapi

import (
	"net/http"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func privateCredentialResponse(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func (api *API) applicationCredentialStatus(w http.ResponseWriter, r *http.Request) {
	privateCredentialResponse(w)
	value, err := api.service.ApplicationCredentialStatus(r.Context(), r.PathValue("productID"))
	commerceResponse(w, r, value, err)
}
func (api *API) resetApplicationSecret(w http.ResponseWriter, r *http.Request) {
	privateCredentialResponse(w)
	var input service.ResetApplicationSecretInput
	if !decode(w, r, &input) {
		return
	}
	value, err := api.service.ResetApplicationSecret(r.Context(), r.PathValue("productID"), adminSessionFrom(r.Context()).UserID, input)
	commerceResponse(w, r, value, err)
}

func (api *API) registerApplicationCredentialRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/app/token", withRateLimit(api.appTokenLimit, func(w http.ResponseWriter, r *http.Request) {
		privateCredentialResponse(w)
		var input service.ApplicationTokenInput
		if !decode(w, r, &input) {
			return
		}
		value, err := api.service.ApplicationToken(r.Context(), input)
		commerceResponse(w, r, value, err)
	}))
	mux.HandleFunc("GET /v1/app/profile", withRateLimit(api.clientReadLimit, func(w http.ResponseWriter, r *http.Request) {
		privateCredentialResponse(w)
		product, err := api.service.AuthenticateApplicationToken(r.Context(), bearerToken(r))
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, map[string]any{"appid": product.ID, "name": product.Name, "bind_mode": product.BindMode, "license_mode": product.LicenseMode})
	}))
	mux.HandleFunc("POST /v1/app/licenses/verify", withRateLimit(api.clientReadLimit, func(w http.ResponseWriter, r *http.Request) {
		privateCredentialResponse(w)
		product, err := api.service.AuthenticateApplicationToken(r.Context(), bearerToken(r))
		if err != nil {
			writeError(w, r, err)
			return
		}
		var input service.VerifyInput
		if !decode(w, r, &input) {
			return
		}
		if input.AppKey != "" && input.AppKey != product.ID {
			writeError(w, r, service.ErrPermissionDenied)
			return
		}
		input.AppKey = product.ID
		input.ClientIP = api.clientIP(r)
		input.UserAgent = r.UserAgent()
		value, err := api.service.Verify(r.Context(), input)
		commerceResponse(w, r, value, err)
	}))
}

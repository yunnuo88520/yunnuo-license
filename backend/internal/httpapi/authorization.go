package httpapi

import (
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"net/http"
)

func (api *API) registerAuthorizationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/cards/requirements", withRateLimit(api.clientWriteLimit, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			AppKey   string `json:"app_key"`
			CardCode string `json:"card_code"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := api.service.CardRequirements(r.Context(), in.AppKey, in.CardCode)
		commerceResponse(w, r, v, e)
	}))
	for _, prefix := range []string{"/admin", "/developer"} {
		read, write := adminReadRoles, adminWriteRoles
		if prefix == "/developer" {
			read = []string{domain.AdminRoleDeveloper}
			write = read
		}
		wrap := func(h http.HandlerFunc, roles []string) http.HandlerFunc {
			if prefix == "/developer" {
				h = api.withDeveloperProduct(h)
			}
			return api.withAdmin(h, roles...)
		}
		mux.HandleFunc("GET "+prefix+"/products/{productID}/authorization-terms", wrap(func(w http.ResponseWriter, r *http.Request) {
			v, e := api.service.AuthorizationTerms(r.Context(), r.PathValue("productID"))
			commerceResponse(w, r, v, e)
		}, read))
		mux.HandleFunc("GET "+prefix+"/products/{productID}/packages", wrap(func(w http.ResponseWriter, r *http.Request) {
			v, e := api.service.ProductPackages(r.Context(), r.PathValue("productID"))
			commerceResponse(w, r, v, e)
		}, read))
		mux.HandleFunc("POST "+prefix+"/products/{productID}/authorization-terms", wrap(func(w http.ResponseWriter, r *http.Request) {
			var in domain.AuthorizationTerms
			if !decode(w, r, &in) {
				return
			}
			v, e := api.service.SetAuthorizationTerms(r.Context(), r.PathValue("productID"), adminSessionFrom(r.Context()).UserID, in)
			commerceResponse(w, r, v, e)
		}, write))
		mux.HandleFunc("GET "+prefix+"/products/{productID}/licenses/{licenseNo}/installations", wrap(func(w http.ResponseWriter, r *http.Request) {
			v, e := api.service.LicenseInstallations(r.Context(), r.PathValue("productID"), r.PathValue("licenseNo"), "")
			commerceResponse(w, r, v, e)
		}, read))
		mux.HandleFunc("POST "+prefix+"/products/{productID}/licenses/{licenseNo}/installations/{instanceID}/disable", wrap(func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Reason string `json:"reason"`
			}
			if !decode(w, r, &in) {
				return
			}
			e := api.service.DisableInstallation(r.Context(), r.PathValue("productID"), r.PathValue("licenseNo"), r.PathValue("instanceID"), "", adminSessionFrom(r.Context()).UserID, in.Reason)
			commerceResponse(w, r, map[string]bool{"disabled": e == nil}, e)
		}, write))
	}
	mux.HandleFunc("POST /v1/licenses/seats/acquire", withRateLimit(api.clientWriteLimit, func(w http.ResponseWriter, r *http.Request) {
		var in service.SeatInput
		if !decode(w, r, &in) {
			return
		}
		in.ClientIP = api.clientIP(r)
		v, e := api.service.AcquireSeat(r.Context(), in)
		commerceResponse(w, r, v, e)
	}))
	mux.HandleFunc("POST /v1/licenses/seats/release", withRateLimit(api.clientWriteLimit, func(w http.ResponseWriter, r *http.Request) {
		var in service.VerifyInput
		if !decode(w, r, &in) {
			return
		}
		in.ClientIP = api.clientIP(r)
		e := api.service.ReleaseSeat(r.Context(), in)
		commerceResponse(w, r, map[string]bool{"accepted": e == nil}, e)
	}))
	mux.HandleFunc("POST /v1/licenses/claim-ticket", withRateLimit(api.clientWriteLimit, func(w http.ResponseWriter, r *http.Request) {
		var in service.VerifyInput
		if !decode(w, r, &in) {
			return
		}
		in.ClientIP = api.clientIP(r)
		v, e := api.service.CreateLicenseClaim(r.Context(), in)
		commerceResponse(w, r, v, e)
	}))
	mux.HandleFunc("POST /user/licenses/claim", api.withCustomer(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Token string `json:"claim_token"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := api.service.RedeemLicenseClaim(r.Context(), customerFrom(r.Context()).ID, in.Token)
		commerceResponse(w, r, v, e)
	}))
	mux.HandleFunc("GET /user/licenses/{licenseNo}/installations", api.withCustomer(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.LicenseInstallations(r.Context(), "", r.PathValue("licenseNo"), customerFrom(r.Context()).ID)
		commerceResponse(w, r, v, e)
	}))
	mux.HandleFunc("POST /user/licenses/{licenseNo}/installations/{instanceID}/disable", api.withCustomer(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		user := customerFrom(r.Context()).ID
		e := api.service.DisableInstallation(r.Context(), "", r.PathValue("licenseNo"), r.PathValue("instanceID"), user, user, in.Reason)
		commerceResponse(w, r, map[string]bool{"disabled": e == nil}, e)
	}))
}

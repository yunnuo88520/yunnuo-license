package httpapi

import (
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"net/http"
)

func (api *API) registerSecurityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/products/{productID}/security-policy", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.SecurityPolicy(r.Context(), r.PathValue("productID"))
		commerceResponse(w, r, v, e)
	}, adminReadRoles...))
	mux.HandleFunc("POST /admin/products/{productID}/security-policy", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in domain.SecurityPolicy
		if !decode(w, r, &in) {
			return
		}
		v, e := api.service.SetSecurityPolicy(r.Context(), r.PathValue("productID"), adminSessionFrom(r.Context()).UserID, in)
		commerceResponse(w, r, v, e)
	}, adminWriteRoles...))
	mux.HandleFunc("GET /admin/security/cases", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.SecurityCases(r.Context(), "")
		commerceResponse(w, r, v, e)
	}, adminReadRoles...))
	mux.HandleFunc("GET /admin/security/appeals", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.SecurityAppeals(r.Context(), "")
		commerceResponse(w, r, v, e)
	}, adminReadRoles...))
	mux.HandleFunc("POST /admin/security/cases/{caseID}/review", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in service.ReviewSecurityInput
		if !decode(w, r, &in) {
			return
		}
		e := api.service.ReviewSecurityCase(r.Context(), r.PathValue("caseID"), adminSessionFrom(r.Context()).UserID, in)
		commerceResponse(w, r, map[string]bool{"saved": e == nil}, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /v1/security/decision", func(w http.ResponseWriter, r *http.Request) {
		var in service.VerifyInput
		if !decode(w, r, &in) {
			return
		}
		in.ClientIP = api.clientIP(r)
		v, e := api.service.SecurityDecision(r.Context(), in)
		commerceResponse(w, r, v, e)
	})
	mux.HandleFunc("POST /v1/security/report", withRateLimit(api.publicQueryLimit, func(w http.ResponseWriter, r *http.Request) {
		var in service.IntegrityReportInput
		if !decode(w, r, &in) {
			return
		}
		in.ClientIP = api.clientIP(r)
		v, e := api.service.ReportIntegrity(r.Context(), in)
		if e == nil {
			v.Evidence = ""
			v.LicenseID = ""
		}
		commerceResponse(w, r, v, e)
	}))
	mux.HandleFunc("GET /user/security/cases", api.withCustomer(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.SecurityCases(r.Context(), customerFrom(r.Context()).ID)
		commerceResponse(w, r, v, e)
	}))
	mux.HandleFunc("GET /user/security/appeals", api.withCustomer(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.SecurityAppeals(r.Context(), customerFrom(r.Context()).ID)
		commerceResponse(w, r, v, e)
	}))
	mux.HandleFunc("POST /user/security/cases/{caseID}/appeal", api.withCustomer(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := api.service.AppealSecurityCase(r.Context(), customerFrom(r.Context()).ID, r.PathValue("caseID"), in.Reason)
		commerceResponse(w, r, v, e)
	}))
}

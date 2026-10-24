package httpapi

import (
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"net/http"
)

func commerceResponse(w http.ResponseWriter, r *http.Request, result any, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) registerCommerceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/commerce", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.CommerceCatalog(r.Context())
		commerceResponse(w, r, v, e)
	}, adminReadRoles...))
	mux.HandleFunc("POST /admin/product-groups", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in domain.ProductGroup
		if !decode(w, r, &in) {
			return
		}
		v, e := api.service.SaveProductGroup(r.Context(), in, adminSessionFrom(r.Context()).UserID)
		commerceResponse(w, r, v, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /admin/packages", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			domain.CardPackage
			Cost *int64 `json:"base_cost"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Cost == nil {
			writeError(w, r, service.ErrInvalidRequest)
			return
		}
		in.CardPackage.BaseCost = *in.Cost
		v, e := api.service.CreatePackage(r.Context(), in.CardPackage, adminSessionFrom(r.Context()).UserID)
		commerceResponse(w, r, v, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /admin/packages/{packageID}", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Enabled  *bool  `json:"enabled"`
			BaseCost *int64 `json:"base_cost"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Enabled == nil || in.BaseCost == nil {
			writeError(w, r, service.ErrInvalidRequest)
			return
		}
		e := api.service.SetPackage(r.Context(), r.PathValue("packageID"), adminSessionFrom(r.Context()).UserID, *in.Enabled, *in.BaseCost)
		commerceResponse(w, r, map[string]bool{"saved": e == nil}, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /admin/agent-tiers", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := api.service.CreateTier(r.Context(), in.Name, adminSessionFrom(r.Context()).UserID)
		commerceResponse(w, r, v, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /admin/agent-tiers/{tierID}/prices", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			PackageID string `json:"package_id"`
			UnitCost  *int64 `json:"unit_cost"`
		}
		if !decode(w, r, &in) {
			return
		}
		e := api.service.SetTierPrice(r.Context(), r.PathValue("tierID"), in.PackageID, adminSessionFrom(r.Context()).UserID, in.UnitCost)
		commerceResponse(w, r, map[string]bool{"saved": e == nil}, e)
	}, adminWriteRoles...))
	mux.HandleFunc("GET /admin/agents/{agentID}/account", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("agentID")
		a, e := api.service.AgentAccount(r.Context(), id)
		if e != nil {
			writeError(w, r, e)
			return
		}
		p, e := api.service.AgentPackages(r.Context(), id)
		if e != nil {
			writeError(w, r, e)
			return
		}
		l, e := api.service.AgentMoneyLedgers(r.Context(), id)
		commerceResponse(w, r, map[string]any{"account": a, "packages": p, "ledgers": l}, e)
	}, adminReadRoles...))
	mux.HandleFunc("POST /admin/agents/{agentID}/tier", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			TierID string `json:"tier_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		e := api.service.SetAgentTier(r.Context(), r.PathValue("agentID"), in.TierID, adminSessionFrom(r.Context()).UserID)
		commerceResponse(w, r, map[string]bool{"saved": e == nil}, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /admin/agents/{agentID}/packages", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in service.GrantPackagesInput
		if !decode(w, r, &in) {
			return
		}
		in.AgentID = r.PathValue("agentID")
		e := api.service.GrantPackages(r.Context(), in, adminSessionFrom(r.Context()).UserID)
		commerceResponse(w, r, map[string]bool{"saved": e == nil}, e)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /admin/agents/{agentID}/balance", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var in service.AdjustBalanceInput
		if !decode(w, r, &in) {
			return
		}
		v, e := api.service.AdjustAgentBalance(r.Context(), r.PathValue("agentID"), adminSessionFrom(r.Context()).UserID, in)
		commerceResponse(w, r, v, e)
	}, adminWriteRoles...))
	mux.HandleFunc("GET /agent/packages", func(w http.ResponseWriter, r *http.Request) {
		a, e := api.agentSession(r)
		if e != nil {
			writeError(w, r, e)
			return
		}
		v, e := api.service.AgentPackages(r.Context(), a.AgentID)
		commerceResponse(w, r, v, e)
	})
	mux.HandleFunc("GET /agent/account", func(w http.ResponseWriter, r *http.Request) {
		a, e := api.agentSession(r)
		if e != nil {
			writeError(w, r, e)
			return
		}
		v, e := api.service.AgentAccount(r.Context(), a.AgentID)
		commerceResponse(w, r, v, e)
	})
	mux.HandleFunc("GET /agent/money-ledgers", func(w http.ResponseWriter, r *http.Request) {
		a, e := api.agentSession(r)
		if e != nil {
			writeError(w, r, e)
			return
		}
		v, e := api.service.AgentMoneyLedgers(r.Context(), a.AgentID)
		commerceResponse(w, r, v, e)
	})
}

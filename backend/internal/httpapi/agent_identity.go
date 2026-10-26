package httpapi

import (
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"net/http"
)

func (api *API) registerAgentIdentityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/agent-accounts/eligible-users", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		users, err := api.service.EligibleAgentUsers(r.Context())
		commerceResponse(w, r, users, err)
	}, adminWriteRoles...))
	mux.HandleFunc("POST /admin/agent-accounts", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		var input service.OpenAgentInput
		if !decode(w, r, &input) {
			return
		}
		result, err := api.service.OpenAgentAccount(r.Context(), input, adminSessionFrom(r.Context()).UserID)
		commerceResponse(w, r, result, err)
	}, adminWriteRoles...))
}

package httpapi

import (
	"net/http"
	"strconv"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func (api *API) registerEmailRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/email-settings", api.withAdmin(api.getEmailSettings, adminReadRoles...))
	mux.HandleFunc("POST /admin/email-settings", api.withAdmin(api.updateEmailSettings, adminWriteRoles...))
	mux.HandleFunc("POST /admin/email-settings/test", api.withAdmin(api.sendTestEmail, adminWriteRoles...))
	mux.HandleFunc("POST /admin/agents/{agentID}/email", api.withAdmin(api.sendAgentEmail, adminWriteRoles...))
	mux.HandleFunc("GET /admin/email-logs", api.withAdmin(api.listEmailLogs, adminReadRoles...))
}

func (api *API) getEmailSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := api.service.EmailSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, settings)
}

func (api *API) updateEmailSettings(w http.ResponseWriter, r *http.Request) {
	var input service.UpdateEmailSettingsInput
	if !decode(w, r, &input) {
		return
	}
	input.ActorID = adminSessionFrom(r.Context()).UserID
	settings, err := api.service.UpdateEmailSettings(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, settings)
}

func (api *API) sendTestEmail(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ToAddress string `json:"to_address"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := api.service.SendTestEmail(r.Context(), adminSessionFrom(r.Context()).UserID, input.ToAddress); err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"sent": true})
}

func (api *API) sendAgentEmail(w http.ResponseWriter, r *http.Request) {
	var input service.SendAgentEmailInput
	if !decode(w, r, &input) {
		return
	}
	input.ActorID = adminSessionFrom(r.Context()).UserID
	recipients, err := api.service.SendAgentEmail(r.Context(), r.PathValue("agentID"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]int{"recipients": recipients})
}

func (api *API) listEmailLogs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	logs, err := api.service.EmailLogs(r.Context(), r.URL.Query().Get("agent_id"), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, logs)
}

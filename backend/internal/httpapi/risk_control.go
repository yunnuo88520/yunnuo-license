package httpapi

import (
	"net/http"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func (api *API) riskSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := api.service.RiskSummary(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, summary)
}

func (api *API) listRiskBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := api.service.ListRiskBlocks(r.Context(), service.RiskBlockFilter{
		Kind:      r.URL.Query().Get("kind"),
		Status:    r.URL.Query().Get("status"),
		ProductID: r.URL.Query().Get("product_id"),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, blocks)
}

func (api *API) createRiskBlock(w http.ResponseWriter, r *http.Request) {
	var input service.CreateRiskBlockInput
	if !decode(w, r, &input) {
		return
	}
	input.ActorID = adminSessionFrom(r.Context()).UserID
	block, err := api.service.CreateRiskBlock(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, block)
}

func (api *API) disableRiskBlock(w http.ResponseWriter, r *http.Request) {
	session := adminSessionFrom(r.Context())
	if err := api.service.DisableRiskBlock(r.Context(), r.PathValue("blockID"), session.UserID); err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]any{"disabled": true})
}

func (api *API) listRiskAlerts(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 100)
	if err != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	alerts, err := api.service.ListRiskAlerts(r.Context(), service.RiskAlertFilter{
		Status:    r.URL.Query().Get("status"),
		Severity:  r.URL.Query().Get("severity"),
		AlertType: r.URL.Query().Get("alert_type"),
		ProductID: r.URL.Query().Get("product_id"),
		Limit:     limit,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, alerts)
}

func (api *API) resolveRiskAlert(w http.ResponseWriter, r *http.Request) {
	session := adminSessionFrom(r.Context())
	if err := api.service.ResolveRiskAlert(r.Context(), r.PathValue("alertID"), session.UserID); err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]any{"resolved": true})
}

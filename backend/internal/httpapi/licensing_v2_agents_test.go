package httpapi

import (
	"net/http"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func TestV2AgentIssuanceUsesAccountTierAndAtomicBalance(t *testing.T) {
	h, root := emailTestHandler(t)
	var app, other core.CreatedApplication
	decodeData(t, performJSON(t, h, "POST", "/v2/console/applications", root, map[string]any{"name": "Agent scope A", "allowed_models": []string{"perpetual"}, "allowed_bindings": []string{"installation"}}), &app)
	decodeData(t, performJSON(t, h, "POST", "/v2/console/applications", root, map[string]any{"name": "Agent scope B", "allowed_models": []string{"perpetual"}, "allowed_bindings": []string{"installation"}}), &other)
	var plan, otherPlan core.Plan
	for _, entry := range []struct {
		app string
		out *core.Plan
	}{{app.AppID, &plan}, {other.AppID, &otherPlan}} {
		decodeData(t, performJSON(t, h, "POST", "/v2/console/applications/"+entry.app+"/plans", root, map[string]any{"name": "Licensed plan", "terms": map[string]any{"model": "perpetual", "binding_types": []string{"installation"}}, "currency": "CNY", "agent_cost_cents": 101}), entry.out)
	}
	var tier domain.AgentTier
	decodeData(t, performJSON(t, h, "POST", "/admin/agent-tiers", root, map[string]any{"name": "V2 half cost", "cost_rate_bps": 5000, "sort_order": 10}), &tier)
	var opened service.OpenAgentResult
	decodeData(t, performJSON(t, h, "POST", "/admin/agent-accounts", root, map[string]any{"username": "v2-seller", "password": "secret123", "initial_balance": 1000, "reason": "manual funding", "tier_id": tier.ID}), &opened)
	prefix := "/v2/console/applications/" + app.AppID
	performJSON(t, h, "POST", prefix+"/agent-grants", root, map[string]any{"agent_id": opened.Agent.ID, "plan_id": plan.ID, "enabled": true, "max_batch_quantity": 10})
	var login service.AgentLoginResponse
	decodeData(t, performJSON(t, h, "POST", "/agent/login", "", map[string]any{"username": "v2-seller", "password": "secret123"}), &login)
	var quotes []core.AgentQuote
	decodeData(t, performJSON(t, h, "GET", "/v2/agent/catalog", login.AccessToken, nil), &quotes)
	if len(quotes) != 1 || quotes[0].Application.ApplicationID != app.AppID || quotes[0].UnitCostCents != 51 {
		t.Fatal("wrong grants or tier rounding", quotes)
	}
	input := map[string]any{"plan_id": plan.ID, "quantity": 2, "customer_ref": "client-1", "request_id": "request-v2-001", "expected_unit_cost_cents": 51}
	var first, replayed core.SaleResult
	decodeData(t, performJSON(t, h, "POST", "/v2/agent/applications/"+app.AppID+"/orders", login.AccessToken, input), &first)
	if len(first.Cards) != 2 || first.Order.CostCents != 102 {
		t.Fatal(first)
	}
	decodeData(t, performJSON(t, h, "POST", "/v2/agent/applications/"+app.AppID+"/orders", login.AccessToken, input), &replayed)
	if !replayed.Replayed || len(replayed.Cards) != 0 {
		t.Fatal("duplicate plaintext delivery", replayed)
	}
	var account domain.AgentAccount
	decodeData(t, performJSON(t, h, "GET", "/agent/account", login.AccessToken, nil), &account)
	if account.Balance != 898 {
		t.Fatal("duplicate debit", account)
	}
	input["quantity"] = 3
	performJSONStatus(t, h, "POST", "/v2/agent/applications/"+app.AppID+"/orders", login.AccessToken, input, http.StatusConflict)
	input["request_id"] = "request-v2-002"
	input["plan_id"] = otherPlan.ID
	performJSONStatus(t, h, "POST", "/v2/agent/applications/"+other.AppID+"/orders", login.AccessToken, input, http.StatusForbidden)
	performJSON(t, h, "POST", prefix+"/agent-grants", root, map[string]any{"agent_id": opened.Agent.ID, "plan_id": plan.ID, "enabled": true, "max_batch_quantity": 10, "override_cost_cents": 88})
	input["plan_id"] = plan.ID
	performJSONStatus(t, h, "POST", "/v2/agent/applications/"+app.AppID+"/orders", login.AccessToken, input, http.StatusConflict)
	decodeData(t, performJSON(t, h, "GET", "/agent/account", login.AccessToken, nil), &account)
	if account.Balance != 898 {
		t.Fatal("rejected sale changed balance", account)
	}
	performJSONStatus(t, h, "POST", prefix+"/agent-grants", login.AccessToken, map[string]any{"agent_id": opened.Agent.ID, "plan_id": plan.ID, "enabled": true, "max_batch_quantity": 10, "override_cost_cents": 0}, http.StatusUnauthorized)
	// Whole-order refunds restore the exact original cost once, independently
	// of subsequent tier/price changes, and revoke the issued entitlements.
	performJSON(t, h, "POST", prefix+"/orders/"+first.Order.ID+"/refund", root, map[string]any{"reason": "verified full refund"})
	performJSON(t, h, "POST", prefix+"/orders/"+first.Order.ID+"/refund", root, map[string]any{"reason": "confirmation retry"})
	decodeData(t, performJSON(t, h, "GET", "/agent/account", login.AccessToken, nil), &account)
	if account.Balance != 1000 {
		t.Fatal("refund cost duplicated or used the changed price", account)
	}
	var records []core.AgentRecords
	decodeData(t, performJSON(t, h, "GET", "/v2/platform/agents/"+opened.Agent.ID+"/records", root, nil), &records)
	if len(records) != 1 || len(records[0].MoneyEvents) != 2 {
		t.Fatal("missing issuance/refund financial audit", records)
	}
	for _, l := range records[0].Licenses {
		if l.Status != "revoked" {
			t.Fatal("refunded license remains usable")
		}
	}

}

package httpapi

import (
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"testing"
)

func TestAgentTierAdministrationPermissionsAndExplicitRate(t *testing.T) {
	h, root := emailTestHandler(t)
	var catalog service.CommerceCatalog
	decodeData(t, performJSON(t, h, "GET", "/admin/commerce", root, nil), &catalog)
	if len(catalog.Tiers) != 3 {
		t.Fatal(catalog.Tiers)
	}
	path := "/admin/agent-tiers/tier_gold"
	input := map[string]any{"name": "金牌合作代理", "cost_rate_bps": 7550, "sort_order": 1}
	performJSONStatus(t, h, "POST", path, root, map[string]any{"name": "Missing rate"}, 400)
	for _, rate := range []int{0, -1, 10001} {
		performJSONStatus(t, h, "POST", path, root, map[string]any{"name": "Bad rate", "cost_rate_bps": rate, "sort_order": 1}, 400)
	}
	var changed domain.AgentTier
	decodeData(t, performJSON(t, h, "POST", path, root, input), &changed)
	if changed.CostRateBPS != 7550 || changed.Name != "金牌合作代理" {
		t.Fatal(changed)
	}
	performJSON(t, h, "POST", "/admin/users", root, map[string]any{"username": "tier-auditor", "password": "secret123", "role": "auditor"})
	auditor := loginAdmin(t, h, "tier-auditor", "secret123")
	performJSONStatus(t, h, "POST", path, auditor, input, 403)
	performJSONStatus(t, h, "DELETE", path, auditor, nil, 403)
	performJSONStatus(t, h, "POST", "/admin/agent-tiers", auditor, input, 403)
	var agent service.OpenAgentResult
	decodeData(t, performJSON(t, h, "POST", "/admin/agent-accounts", root, map[string]any{"username": "tier-seller", "password": "secret123", "tier_id": "tier_gold"}), &agent)
	var login service.AgentLoginResponse
	decodeData(t, performJSON(t, h, "POST", "/agent/login", "", map[string]any{"username": "tier-seller", "password": "secret123"}), &login)
	performJSONStatus(t, h, "POST", path, login.AccessToken, input, 401)
	performJSONStatus(t, h, "DELETE", path, root, nil, 409)
	performJSONStatus(t, h, "POST", "/admin/agent-tiers/missing", root, input, 404)
	performJSON(t, h, "POST", "/admin/agents/"+agent.Agent.ID+"/tier", root, map[string]any{"tier_id": "tier_standard"})
	performJSON(t, h, "DELETE", path, root, nil)
	performJSONStatus(t, h, "DELETE", path, root, nil, 404)
}

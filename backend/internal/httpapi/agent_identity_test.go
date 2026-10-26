package httpapi

import (
	"net/http"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func TestUnifiedAgentAccountScopeAndCredentialLifecycle(t *testing.T) {
	h, root := emailTestHandler(t)
	var existing domain.Customer
	decodeData(t, performJSON(t, h, "POST", "/admin/customers", root, map[string]any{"username": "seller", "password": "secret123", "email": "seller@example.com"}), &existing)
	var normal service.CustomerLoginResponse
	decodeData(t, performJSON(t, h, "POST", "/user/login", "", map[string]any{"username": "seller@example.com", "password": "secret123"}), &normal)
	performJSONStatus(t, h, "GET", "/agent/profile", normal.AccessToken, nil, 403)
	var result service.OpenAgentResult
	decodeData(t, performJSON(t, h, "POST", "/admin/agent-accounts", root, map[string]any{"user_id": existing.ID, "name": "Seller Agency", "initial_balance": 1000, "reason": "Opening credit"}), &result)
	if result.User.ID != existing.ID || result.Agent.OwnerUserID != existing.ID || result.Account.Balance != 1000 {
		t.Fatalf("incorrect binding: %+v", result)
	}
	// A session created before the upgrade gains only its own qualification.
	var scope service.AgentSession
	decodeData(t, performJSON(t, h, "GET", "/agent/profile", normal.AccessToken, nil), &scope)
	if scope.AgentID != result.Agent.ID {
		t.Fatal(scope)
	}
	performJSONStatus(t, h, "GET", "/admin/agents", normal.AccessToken, nil, 401)
	performJSONStatus(t, h, "POST", "/admin/agent-accounts", normal.AccessToken, map[string]any{"user_id": existing.ID}, 401)
	performJSONStatus(t, h, "POST", "/admin/agent-accounts", root, map[string]any{"user_id": existing.ID, "initial_balance": 9999, "reason": "Duplicate"}, 409)
	var ledger []domain.MoneyLedger
	decodeData(t, performJSON(t, h, "GET", "/agent/money-ledgers", normal.AccessToken, nil), &ledger)
	if len(ledger) != 1 || ledger[0].Amount != 1000 {
		t.Fatal("duplicate opening credit", ledger)
	}
	var login service.AgentLoginResponse
	decodeData(t, performJSON(t, h, "POST", "/agent/login", "", map[string]any{"username": "seller", "password": "secret123"}), &login)
	var profile domain.Customer
	decodeData(t, performJSON(t, h, "GET", "/user/profile", login.AccessToken, nil), &profile)
	if profile.ID != existing.ID || profile.AgentID != result.Agent.ID {
		t.Fatal(profile)
	}
	var logs struct {
		Items []domain.OperationLog `json:"items"`
	}
	decodeData(t, performJSON(t, h, "GET", "/admin/operation-logs?page_size=500", root, nil), &logs)
	userScope, agentScope := false, false
	for _, l := range logs.Items {
		userScope = userScope || l.Route == "GET /user/profile" && l.ActorType == "customer" && l.ActorID == existing.ID
		agentScope = agentScope || l.Route == "GET /agent/profile" && l.ActorType == "agent" && l.ActorID == existing.ID
	}
	if !userScope || !agentScope {
		t.Fatal("shared identity audit lost endpoint scope", logs)
	}
	// Suspending business rights never disables the underlying customer identity.
	performJSON(t, h, "POST", "/admin/agents/"+result.Agent.ID+"/suspend", root, nil)
	performJSONStatus(t, h, "GET", "/agent/profile", login.AccessToken, nil, 403)
	performJSON(t, h, "GET", "/user/profile", login.AccessToken, nil)
	performJSON(t, h, "POST", "/admin/agents/"+result.Agent.ID+"/enable", root, nil)
	performJSON(t, h, "POST", "/user/password", login.AccessToken, map[string]any{"current_password": "secret123", "new_password": "changed123"})
	for _, path := range []string{"/user/profile", "/agent/profile"} {
		performJSONStatus(t, h, "GET", path, login.AccessToken, nil, 401)
	}
	performJSONStatus(t, h, "POST", "/agent/login", "", map[string]any{"username": "seller", "password": "secret123"}, 401)
	decodeData(t, performJSON(t, h, "POST", "/agent/login", "", map[string]any{"username": "seller", "password": "changed123"}), &login)
	// Recovery through the agent entry resets the same personal identity and revokes both portals.
	performJSON(t, h, "POST", "/admin/recovery/agent/"+login.User.ID+"/password", root, map[string]any{"password": "recovered123", "reason": "Verified owner"})
	for _, path := range []string{"/user/profile", "/agent/profile"} {
		performJSONStatus(t, h, "GET", path, login.AccessToken, nil, 401)
	}
	decodeData(t, performJSON(t, h, "POST", "/user/login", "", map[string]any{"username": "seller", "password": "recovered123"}), &normal)
	performJSON(t, h, "GET", "/agent/profile", normal.AccessToken, nil)
	performJSON(t, h, "POST", "/admin/customers/"+existing.ID+"/disable", root, nil)
	for _, path := range []string{"/user/profile", "/agent/profile"} {
		performJSONStatus(t, h, "GET", path, normal.AccessToken, nil, 401)
	}
}

func TestOpenAgentAccountAtomicValidationAndGlobalLogins(t *testing.T) {
	h, root := emailTestHandler(t)
	request := map[string]any{"username": "new-seller", "password": "secret123", "initial_balance": 1000, "reason": "Opening credit", "tier_id": "missing-tier"}
	performJSONStatus(t, h, "POST", "/admin/agent-accounts", root, request, 400)
	var users []domain.Customer
	var agents []domain.Agent
	decodeData(t, performJSON(t, h, "GET", "/admin/customers", root, nil), &users)
	decodeData(t, performJSON(t, h, "GET", "/admin/agents", root, nil), &agents)
	if len(users) != 0 || len(agents) != 0 {
		t.Fatal("invalid tier created partial accounts", users, agents)
	}
	delete(request, "tier_id")
	request["package_ids"] = []string{"missing-package"}
	performJSONStatus(t, h, "POST", "/admin/agent-accounts", root, request, 403)
	delete(request, "package_ids")
	request["email"] = "new-seller@example.com"
	var opened service.OpenAgentResult
	decodeData(t, performJSON(t, h, "POST", "/admin/agent-accounts", root, request), &opened)
	performJSONStatus(t, h, "POST", "/admin/agent-accounts", root, request, 409)
	performJSONStatus(t, h, "POST", "/admin/customers", root, map[string]any{"username": "new-seller@example.com", "password": "secret123"}, 409)
	var other service.OpenAgentResult
	decodeData(t, performJSON(t, h, "POST", "/admin/agent-accounts", root, map[string]any{"username": "other", "password": "secret123"}), &other)
	var login service.AgentLoginResponse
	decodeData(t, performJSON(t, h, "POST", "/agent/login", "", map[string]any{"username": "other", "password": "secret123"}), &login)
	var account domain.AgentAccount
	decodeData(t, performJSON(t, h, "GET", "/agent/account?agent_id="+opened.Agent.ID, login.AccessToken, nil), &account)
	if account.AgentID != other.Agent.ID {
		t.Fatal("cross agency access", account)
	}
	// A business contact address must not become a personal password recovery address.
	performJSON(t, h, "POST", "/admin/agents/"+other.Agent.ID+"/update", root, map[string]any{"name": "Other", "email": "company@example.com"})
	performJSONStatus(t, h, http.MethodPost, "/admin/recovery/agent/"+other.User.ID+"/email", root, map[string]any{"reason": "No personal email"}, 400)
}

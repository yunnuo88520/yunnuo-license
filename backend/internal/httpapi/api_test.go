package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func TestStaticAppServesAssetsAndRejectsMissingFiles(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<link rel=\"stylesheet\" href=\"/styles.css\">"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "styles.css"), []byte("body { color: green; }"), 0o600); err != nil {
		t.Fatal(err)
	}

	handler := New(newHTTPTestService(t), staticDir, "", "").Handler()
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/styles.css", nil))
	if asset.Code != http.StatusOK || !strings.Contains(asset.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("expected CSS asset, got status=%d content-type=%q", asset.Code, asset.Header().Get("Content-Type"))
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/missing.css", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected missing asset to return 404, got %d", missing.Code)
	}

	fallback := httptest.NewRecorder()
	handler.ServeHTTP(fallback, httptest.NewRequest(http.MethodGet, "/lookup/result", nil))
	if fallback.Code != http.StatusOK || !strings.Contains(fallback.Body.String(), "styles.css") {
		t.Fatalf("expected app fallback, got status=%d body=%q", fallback.Code, fallback.Body.String())
	}
}

func TestAgentBearerTokenCanGenerateCards(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t)
	product, err := svc.CreateProduct(ctx, service.CreateProductInput{
		Name:                 "Test Product",
		Code:                 "YN",
		BindMode:             domain.BindDevice,
		MaxBindCount:         1,
		BindConflictStrategy: domain.ConflictReject,
		OfflineGraceDays:     15,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	agent, err := svc.CreateAgent(ctx, service.CreateAgentInput{Name: "Bearer 代理"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.CreateAgentUser(ctx, service.CreateAgentUserInput{
		AgentID:  agent.ID,
		Username: "owner",
		Password: "secret123",
	}); err != nil {
		t.Fatalf("create agent user: %v", err)
	}
	if _, err := svc.CreateAgentUser(ctx, service.CreateAgentUserInput{
		AgentID:  agent.ID,
		Username: "reader",
		Password: "secret123",
		Role:     domain.AgentRoleReadonly,
	}); err != nil {
		t.Fatalf("create readonly agent user: %v", err)
	}
	if _, err := svc.UpsertAgentProductPolicy(ctx, service.AgentProductPolicyInput{
		AgentID:             agent.ID,
		ProductID:           product.ID,
		CanGenerate:         true,
		CanExportPlainCode:  true,
		AllowedDurationDays: []int{30},
		MaxBatchQuantity:    10,
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	if _, err := svc.GrantAgentQuota(ctx, service.AgentQuotaInput{
		AgentID:      agent.ID,
		ProductID:    product.ID,
		DurationDays: 30,
		Quantity:     3,
	}); err != nil {
		t.Fatalf("grant quota: %v", err)
	}

	handler := New(svc, "", "", "").Handler()
	loginBody := map[string]any{
		"login_code": agent.LoginCode,
		"username":   "owner",
		"password":   "secret123",
	}
	loginResp := performJSON(t, handler, http.MethodPost, "/agent/login", "", loginBody)
	var login service.AgentLoginResponse
	decodeData(t, loginResp, &login)
	if login.AccessToken == "" {
		t.Fatal("expected access token")
	}
	performJSONStatus(t, handler, http.MethodGet, "/admin/products", login.AccessToken, nil, http.StatusUnauthorized)

	batchBody := map[string]any{
		"product_id":    product.ID,
		"name":          "token batch",
		"quantity":      2,
		"duration_days": 30,
	}
	batchResp := performJSON(t, handler, http.MethodPost, "/agent/card-batches", login.AccessToken, batchBody)
	var generated service.CreateCardBatchResult
	decodeData(t, batchResp, &generated)
	if generated.Batch.AgentID != agent.ID || len(generated.Codes) != 2 {
		t.Fatalf("unexpected generated batch: %#v", generated)
	}
	batchesResp := performJSON(t, handler, http.MethodGet, "/agent/card-batches", login.AccessToken, nil)
	var batches []domain.CardBatch
	decodeData(t, batchesResp, &batches)
	if len(batches) != 1 || batches[0].ID != generated.Batch.ID {
		t.Fatalf("unexpected agent batches: %#v", batches)
	}
	cardsResp := performJSON(t, handler, http.MethodGet, "/agent/card-batches/"+generated.Batch.ID+"/cards", login.AccessToken, nil)
	var cards []domain.Card
	decodeData(t, cardsResp, &cards)
	if len(cards) != 2 {
		t.Fatalf("expected 2 scoped cards, got %#v", cards)
	}
	exportResp := performJSON(t, handler, http.MethodPost, "/agent/card-batches/"+generated.Batch.ID+"/export", login.AccessToken, nil)
	var exported service.AgentCardExportResult
	decodeData(t, exportResp, &exported)
	if len(exported.Codes) != 2 {
		t.Fatalf("expected 2 exported codes, got %#v", exported)
	}

	quotaResp := performJSON(t, handler, http.MethodGet, "/agent/quotas", login.AccessToken, nil)
	var quotas []domain.AgentQuotaSummary
	decodeData(t, quotaResp, &quotas)
	if len(quotas) != 1 || quotas[0].Balance != 1 {
		t.Fatalf("expected remaining quota 1, got %#v", quotas)
	}

	readonlyLoginResp := performJSON(t, handler, http.MethodPost, "/agent/login", "", map[string]any{
		"agent_no": agent.AgentNo,
		"username": "reader",
		"password": "secret123",
	})
	var readonlyLogin service.AgentLoginResponse
	decodeData(t, readonlyLoginResp, &readonlyLogin)
	performJSONStatus(t, handler, http.MethodPost, "/agent/card-batches", readonlyLogin.AccessToken, batchBody, http.StatusForbidden)
	performJSONStatus(t, handler, http.MethodPost, "/agent/card-batches/"+generated.Batch.ID+"/export", readonlyLogin.AccessToken, nil, http.StatusForbidden)

	otherAgent, err := svc.CreateAgent(ctx, service.CreateAgentInput{Name: "Other Agent"})
	if err != nil {
		t.Fatalf("create other agent: %v", err)
	}
	if _, err := svc.CreateAgentUser(ctx, service.CreateAgentUserInput{
		AgentID:  otherAgent.ID,
		Username: "owner",
		Password: "secret123",
	}); err != nil {
		t.Fatalf("create other agent user: %v", err)
	}
	otherLoginResp := performJSON(t, handler, http.MethodPost, "/agent/login", "", map[string]any{
		"login_code": otherAgent.LoginCode,
		"username":   "owner",
		"password":   "secret123",
	})
	var otherLogin service.AgentLoginResponse
	decodeData(t, otherLoginResp, &otherLogin)
	performJSONStatus(t, handler, http.MethodGet, "/agent/card-batches/"+generated.Batch.ID+"/cards", otherLogin.AccessToken, nil, http.StatusNotFound)
}

func TestAdminAuthenticationAndRoleBoundaries(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t)
	_, created, err := svc.EnsureBootstrapAdmin(ctx, "root", "secret123", "Root Admin")
	if err != nil || !created {
		t.Fatalf("bootstrap admin: created=%v err=%v", created, err)
	}
	handler := New(svc, "", "", "").Handler()

	performJSONStatus(t, handler, http.MethodGet, "/admin/products", "", nil, http.StatusUnauthorized)
	rootToken := loginAdmin(t, handler, "root", "secret123")

	productResp := performJSON(t, handler, http.MethodPost, "/admin/products", rootToken, map[string]any{
		"name":                   "Admin Product",
		"code":                   "ADM",
		"bind_mode":              domain.BindDevice,
		"max_bind_count":         1,
		"bind_conflict_strategy": domain.ConflictReject,
		"offline_grace_days":     15,
		"expire_grace_days":      3,
		"unbind_limit":           3,
		"unbind_cooldown_hours":  24,
	})
	var product domain.Product
	decodeData(t, productResp, &product)

	for _, user := range []map[string]any{
		{"username": "operator", "password": "operator123", "display_name": "Operator", "role": domain.AdminRoleOperator},
		{"username": "auditor", "password": "auditor123", "display_name": "Auditor", "role": domain.AdminRoleAuditor},
	} {
		performJSON(t, handler, http.MethodPost, "/admin/users", rootToken, user)
	}

	operatorToken := loginAdmin(t, handler, "operator", "operator123")
	performJSON(t, handler, http.MethodGet, "/admin/products", operatorToken, nil)
	performJSONStatus(t, handler, http.MethodPost, "/admin/products", operatorToken, map[string]any{
		"name": "Denied Product",
		"code": "DENY",
	}, http.StatusForbidden)
	operatorBatchResp := performJSON(t, handler, http.MethodPost, "/admin/card-batches", operatorToken, map[string]any{
		"product_id":    product.ID,
		"name":          "Operator Batch",
		"quantity":      1,
		"duration_days": 30,
	})
	var operatorBatch service.CreateCardBatchResult
	decodeData(t, operatorBatchResp, &operatorBatch)
	cardsResp := performJSON(t, handler, http.MethodGet, "/admin/card-batches/"+operatorBatch.Batch.ID+"/cards", operatorToken, nil)
	var cards []domain.Card
	decodeData(t, cardsResp, &cards)
	if len(cards) != 1 {
		t.Fatalf("expected operator batch card, got %#v", cards)
	}
	performJSON(t, handler, http.MethodPost, "/admin/card-batches/"+operatorBatch.Batch.ID+"/export", operatorToken, nil)
	performJSONStatus(t, handler, http.MethodPost, "/admin/cards/void", operatorToken, map[string]any{
		"card_id": cards[0].ID,
		"reason":  "not allowed",
	}, http.StatusForbidden)
	performJSON(t, handler, http.MethodPost, "/admin/cards/void", rootToken, map[string]any{
		"card_id": cards[0].ID,
		"reason":  "duplicate_generation",
	})
	performJSON(t, handler, http.MethodPost, "/admin/products/"+product.ID+"/disable", rootToken, nil)
	performJSONStatus(t, handler, http.MethodPost, "/admin/card-batches", operatorToken, map[string]any{
		"product_id":    product.ID,
		"name":          "Disabled Product Batch",
		"quantity":      1,
		"duration_days": 30,
	}, http.StatusForbidden)
	performJSON(t, handler, http.MethodPost, "/admin/products/"+product.ID+"/enable", rootToken, nil)
	performJSONStatus(t, handler, http.MethodPost, "/admin/offline-licenses", operatorToken, map[string]any{
		"product_id":    product.ID,
		"machine_code":  "OPERATOR-MACHINE",
		"duration_days": 30,
	}, http.StatusForbidden)
	offlineResp := performJSON(t, handler, http.MethodPost, "/admin/offline-licenses", rootToken, map[string]any{
		"product_id":    product.ID,
		"machine_code":  "ADMIN-MACHINE-123456",
		"label":         "Offline customer",
		"duration_days": 365,
	})
	var offlineLicense domain.OfflineLicense
	decodeData(t, offlineResp, &offlineLicense)
	if bytes.Contains(offlineResp, []byte("ADMIN-MACHINE-123456")) {
		t.Fatalf("offline create response leaked machine code: %s", offlineResp)
	}
	downloadRec := performJSONRequest(t, handler, http.MethodGet, "/admin/offline-licenses/"+offlineLicense.ID+"/download", rootToken, nil)
	if downloadRec.Code != http.StatusOK || downloadRec.Header().Get("Content-Disposition") == "" || !bytes.Contains(downloadRec.Body.Bytes(), []byte("yn-license-key")) {
		t.Fatalf("unexpected offline download: status=%d headers=%v body=%s", downloadRec.Code, downloadRec.Header(), downloadRec.Body.String())
	}

	auditorToken := loginAdmin(t, handler, "auditor", "auditor123")
	performJSON(t, handler, http.MethodGet, "/admin/licenses", auditorToken, nil)
	performJSON(t, handler, http.MethodGet, "/admin/offline-licenses", auditorToken, nil)
	performJSONStatus(t, handler, http.MethodGet, "/admin/offline-licenses/"+offlineLicense.ID+"/download", auditorToken, nil, http.StatusForbidden)
	performJSONStatus(t, handler, http.MethodPost, "/admin/card-batches", auditorToken, map[string]any{
		"product_id":    product.ID,
		"name":          "Denied Batch",
		"quantity":      1,
		"duration_days": 30,
	}, http.StatusForbidden)
	performJSON(t, handler, http.MethodPost, "/admin/offline-licenses/"+offlineLicense.ID+"/revoke", rootToken, map[string]any{
		"reason": "deployment_retired",
	})
	performJSONStatus(t, handler, http.MethodGet, "/admin/offline-licenses/"+offlineLicense.ID+"/download", rootToken, nil, http.StatusForbidden)

	performJSON(t, handler, http.MethodPost, "/admin/password", rootToken, map[string]any{
		"current_password": "secret123",
		"new_password":     "new-secret123",
	})
	performJSONStatus(t, handler, http.MethodGet, "/admin/profile", rootToken, nil, http.StatusUnauthorized)
	loginAdmin(t, handler, "root", "new-secret123")
}

func TestPublicLicenseQueryDoesNotRequireAuthentication(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t)
	product, err := svc.CreateProduct(ctx, service.CreateProductInput{
		Name:                 "Public Query Product",
		Code:                 "PUB",
		BindMode:             domain.BindDevice,
		MaxBindCount:         1,
		BindConflictStrategy: domain.ConflictReject,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	batch, err := svc.CreateCardBatch(ctx, service.CreateCardBatchInput{
		ProductID:    product.ID,
		Name:         "public query",
		Quantity:     1,
		DurationDays: 30,
	})
	if err != nil {
		t.Fatalf("create card batch: %v", err)
	}
	handler := New(svc, "", "", "").Handler()
	body := performJSON(t, handler, http.MethodPost, "/v1/licenses/query", "", map[string]any{
		"card_code": batch.Codes[0],
	})
	var result service.PublicLicenseQueryResult
	decodeData(t, body, &result)
	if result.ProductName != product.Name || result.CardStatus != domain.CardUnused {
		t.Fatalf("unexpected public query result: %#v", result)
	}
}

func TestProductKeyRingIsPublicAndRotationRequiresSuperAdmin(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t)
	if _, _, err := svc.EnsureBootstrapAdmin(ctx, "root", "secret123", "Root Admin"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	if _, err := svc.CreateAdminUser(ctx, service.CreateAdminUserInput{
		Username: "manager", Password: "secret123", Role: domain.AdminRoleAdmin,
	}); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	product, err := svc.CreateProduct(ctx, service.CreateProductInput{Name: "Key Product", Code: "KEY"})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	handler := New(svc, "", "", "").Handler()

	publicBody := performJSON(t, handler, http.MethodGet, "/v1/products/keys?app_key="+product.AppKey, "", nil)
	var publicRing service.ProductKeyRing
	decodeData(t, publicBody, &publicRing)
	if publicRing.CurrentVersion != 1 || len(publicRing.Keys) != 1 || publicRing.Keys[0].PublicKeyPEM == "" {
		t.Fatalf("unexpected public key ring: %#v", publicRing)
	}
	if bytes.Contains(publicBody, []byte("created_by")) {
		t.Fatalf("public key ring leaked internal actor metadata: %s", publicBody)
	}

	managerToken := loginAdmin(t, handler, "manager", "secret123")
	performJSONStatus(t, handler, http.MethodPost, "/admin/products/"+product.ID+"/keys/rotate", managerToken, nil, http.StatusForbidden)

	rootToken := loginAdmin(t, handler, "root", "secret123")
	rotatedBody := performJSON(t, handler, http.MethodPost, "/admin/products/"+product.ID+"/keys/rotate", rootToken, nil)
	var rotated service.ProductKeyRing
	decodeData(t, rotatedBody, &rotated)
	if rotated.CurrentVersion != 2 || len(rotated.Keys) != 2 {
		t.Fatalf("unexpected rotated key ring: %#v", rotated)
	}
}

func TestAdminCanControlAgentAndUserStatus(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t)
	if _, _, err := svc.EnsureBootstrapAdmin(ctx, "root", "secret123", "Root Admin"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	agent, err := svc.CreateAgent(ctx, service.CreateAgentInput{Name: "Lifecycle Agent"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	user, err := svc.CreateAgentUser(ctx, service.CreateAgentUserInput{
		AgentID:  agent.ID,
		Username: "owner",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("create agent user: %v", err)
	}
	handler := New(svc, "", "", "").Handler()
	adminToken := loginAdmin(t, handler, "root", "secret123")
	agentLoginBody := map[string]any{
		"login_code": agent.LoginCode,
		"username":   "owner",
		"password":   "secret123",
	}
	agentLoginResp := performJSON(t, handler, http.MethodPost, "/agent/login", "", agentLoginBody)
	var agentLogin service.AgentLoginResponse
	decodeData(t, agentLoginResp, &agentLogin)

	performJSON(t, handler, http.MethodPost, "/admin/agents/"+agent.ID+"/users/"+user.ID+"/disable", adminToken, nil)
	performJSONStatus(t, handler, http.MethodGet, "/agent/profile", agentLogin.AccessToken, nil, http.StatusForbidden)
	performJSON(t, handler, http.MethodPost, "/admin/agents/"+agent.ID+"/users/"+user.ID+"/enable", adminToken, nil)
	agentLoginResp = performJSON(t, handler, http.MethodPost, "/agent/login", "", agentLoginBody)
	decodeData(t, agentLoginResp, &agentLogin)

	performJSON(t, handler, http.MethodPost, "/admin/agents/"+agent.ID+"/suspend", adminToken, nil)
	performJSONStatus(t, handler, http.MethodGet, "/agent/profile", agentLogin.AccessToken, nil, http.StatusForbidden)
	performJSON(t, handler, http.MethodPost, "/admin/agents/"+agent.ID+"/enable", adminToken, nil)
	performJSON(t, handler, http.MethodPost, "/agent/login", "", agentLoginBody)

	auditBody := performJSON(t, handler, http.MethodGet, "/admin/audit-logs?action=status", adminToken, nil)
	var logs []domain.AuditLog
	decodeData(t, auditBody, &logs)
	if len(logs) < 4 {
		t.Fatalf("expected lifecycle audit logs, got %#v", logs)
	}
}

func loginAdmin(t *testing.T, handler http.Handler, username, password string) string {
	t.Helper()
	body := performJSON(t, handler, http.MethodPost, "/admin/login", "", map[string]any{
		"username": username,
		"password": password,
	})
	var login service.AdminLoginResponse
	decodeData(t, body, &login)
	if login.AccessToken == "" {
		t.Fatal("expected admin access token")
	}
	return login.AccessToken
}

func newHTTPTestService(t *testing.T) *service.Service {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, "sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	if err := st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return service.New(st, []byte("test-card-pepper"), []byte("0123456789abcdef0123456789abcdef"))
}

func performJSON(t *testing.T, handler http.Handler, method, path, token string, body any) []byte {
	t.Helper()
	rec := performJSONRequest(t, handler, method, path, token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s returned %d: %s", method, path, rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func performJSONStatus(t *testing.T, handler http.Handler, method, path, token string, body any, expected int) {
	t.Helper()
	rec := performJSONRequest(t, handler, method, path, token, body)
	if rec.Code != expected {
		t.Fatalf("%s %s returned %d, expected %d: %s", method, path, rec.Code, expected, rec.Body.String())
	}
}

func performJSONRequest(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeData(t *testing.T, body []byte, target any) {
	t.Helper()
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("expected success response: %s", body)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatalf("decode data: %v", err)
	}
}

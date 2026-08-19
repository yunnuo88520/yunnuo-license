package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

var (
	adminReadRoles = []string{
		domain.AdminRoleSuperAdmin,
		domain.AdminRoleAdmin,
		domain.AdminRoleOperator,
		domain.AdminRoleAuditor,
	}
	adminWriteRoles = []string{
		domain.AdminRoleSuperAdmin,
		domain.AdminRoleAdmin,
	}
	adminOperateRoles = []string{
		domain.AdminRoleSuperAdmin,
		domain.AdminRoleAdmin,
		domain.AdminRoleOperator,
	}
	adminSuperRoles = []string{domain.AdminRoleSuperAdmin}
)

type API struct {
	service           *service.Service
	publicStaticDir   string
	adminStaticDir    string
	agentStaticDir    string
	adminLoginLimit   *fixedWindowLimiter
	agentLoginLimit   *fixedWindowLimiter
	publicQueryLimit  *fixedWindowLimiter
	trustProxyHeaders bool
}

func (api *API) WithTrustedProxyHeaders(enabled bool) *API {
	api.trustProxyHeaders = enabled
	return api
}

func New(svc *service.Service, publicStaticDir, adminStaticDir, agentStaticDir string) *API {
	return &API{
		service:          svc,
		publicStaticDir:  publicStaticDir,
		adminStaticDir:   adminStaticDir,
		agentStaticDir:   agentStaticDir,
		adminLoginLimit:  newFixedWindowLimiter(20, 5*time.Minute),
		agentLoginLimit:  newFixedWindowLimiter(30, 5*time.Minute),
		publicQueryLimit: newFixedWindowLimiter(60, time.Minute),
	}
}

func (api *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("POST /admin/login", withRateLimit(api.adminLoginLimit, api.adminLogin))
	mux.HandleFunc("GET /admin/profile", api.withAdmin(api.adminProfile, adminReadRoles...))
	mux.HandleFunc("POST /admin/password", api.withAdmin(api.changeAdminPassword, adminReadRoles...))
	mux.HandleFunc("GET /admin/users", api.withAdmin(api.listAdminUsers, adminSuperRoles...))
	mux.HandleFunc("POST /admin/users", api.withAdmin(api.createAdminUser, adminSuperRoles...))
	mux.HandleFunc("GET /admin/products", api.withAdmin(api.listProducts, adminReadRoles...))
	mux.HandleFunc("POST /admin/products", api.withAdmin(api.createProduct, adminWriteRoles...))
	mux.HandleFunc("GET /admin/products/{productID}/keys", api.withAdmin(api.productKeys, adminReadRoles...))
	mux.HandleFunc("POST /admin/products/{productID}/keys/rotate", api.withAdmin(api.rotateProductKey, adminSuperRoles...))
	mux.HandleFunc("POST /admin/products/", api.withAdmin(api.productDetail, adminWriteRoles...))
	mux.HandleFunc("GET /admin/agents", api.withAdmin(api.listAgents, adminReadRoles...))
	mux.HandleFunc("POST /admin/agents", api.withAdmin(api.createAgent, adminWriteRoles...))
	mux.HandleFunc("GET /admin/agents/", api.withAdmin(api.agentDetail, adminReadRoles...))
	mux.HandleFunc("POST /admin/agents/", api.withAdmin(api.agentDetail, adminWriteRoles...))
	mux.HandleFunc("POST /admin/agent-policies", api.withAdmin(api.upsertAgentPolicy, adminWriteRoles...))
	mux.HandleFunc("POST /admin/agent-quotas/grant", api.withAdmin(api.grantAgentQuota, adminWriteRoles...))
	mux.HandleFunc("GET /admin/card-batches", api.withAdmin(api.listCardBatches, adminReadRoles...))
	mux.HandleFunc("POST /admin/card-batches", api.withAdmin(api.createCardBatch, adminOperateRoles...))
	mux.HandleFunc("GET /admin/card-batches/", api.withAdmin(api.cardBatchDetail, adminReadRoles...))
	mux.HandleFunc("POST /admin/card-batches/", api.withAdmin(api.cardBatchDetail, adminOperateRoles...))
	mux.HandleFunc("POST /admin/cards/void", api.withAdmin(api.voidCard, adminWriteRoles...))
	mux.HandleFunc("GET /admin/licenses", api.withAdmin(api.listLicenses, adminReadRoles...))
	mux.HandleFunc("POST /admin/licenses/revoke", api.withAdmin(api.revokeLicense, adminWriteRoles...))
	mux.HandleFunc("POST /admin/licenses/unbind", api.withAdmin(api.adminUnbind, adminWriteRoles...))
	mux.HandleFunc("GET /admin/licenses/", api.withAdmin(api.licenseDetail, adminReadRoles...))
	mux.HandleFunc("GET /admin/offline-licenses", api.withAdmin(api.listOfflineLicenses, adminReadRoles...))
	mux.HandleFunc("POST /admin/offline-licenses", api.withAdmin(api.createOfflineLicense, adminWriteRoles...))
	mux.HandleFunc("GET /admin/offline-licenses/", api.withAdmin(api.offlineLicenseDetail, adminWriteRoles...))
	mux.HandleFunc("POST /admin/offline-licenses/", api.withAdmin(api.offlineLicenseDetail, adminWriteRoles...))
	mux.HandleFunc("GET /admin/risk/summary", api.withAdmin(api.riskSummary, adminReadRoles...))
	mux.HandleFunc("GET /admin/risk/blocks", api.withAdmin(api.listRiskBlocks, adminReadRoles...))
	mux.HandleFunc("POST /admin/risk/blocks", api.withAdmin(api.createRiskBlock, adminWriteRoles...))
	mux.HandleFunc("POST /admin/risk/blocks/{blockID}/disable", api.withAdmin(api.disableRiskBlock, adminWriteRoles...))
	mux.HandleFunc("GET /admin/risk/alerts", api.withAdmin(api.listRiskAlerts, adminReadRoles...))
	mux.HandleFunc("POST /admin/risk/alerts/{alertID}/resolve", api.withAdmin(api.resolveRiskAlert, adminWriteRoles...))
	mux.HandleFunc("GET /admin/audit-logs", api.withAdmin(api.listAuditLogs, adminReadRoles...))
	mux.HandleFunc("POST /v1/licenses/activate", api.activate)
	mux.HandleFunc("POST /v1/licenses/verify", api.verify)
	mux.HandleFunc("POST /v1/licenses/heartbeat", api.heartbeat)
	mux.HandleFunc("POST /v1/licenses/renew", api.renew)
	mux.HandleFunc("POST /v1/licenses/unbind", api.unbind)
	mux.HandleFunc("POST /v1/licenses/query", withRateLimit(api.publicQueryLimit, api.publicLicenseQuery))
	mux.HandleFunc("GET /v1/products/keys", withRateLimit(api.publicQueryLimit, api.publicProductKeys))
	mux.HandleFunc("POST /agent/login", withRateLimit(api.agentLoginLimit, api.agentLogin))
	mux.HandleFunc("GET /agent/profile", api.agentProfile)
	mux.HandleFunc("GET /agent/products", api.agentProducts)
	mux.HandleFunc("GET /agent/quotas", api.agentQuotas)
	mux.HandleFunc("GET /agent/quota-ledgers", api.agentQuotaLedgers)
	mux.HandleFunc("GET /agent/card-batches", api.agentCardBatches)
	mux.HandleFunc("POST /agent/card-batches", api.agentCreateCardBatch)
	mux.HandleFunc("GET /agent/card-batches/", api.agentCardBatchDetail)
	mux.HandleFunc("POST /agent/card-batches/", api.agentCardBatchDetail)
	mux.HandleFunc("GET /agent/licenses", api.agentLicenses)
	mux.HandleFunc("GET /agent-console", api.agentAppRedirect)
	mux.HandleFunc("GET /agent-console/", api.agentApp)
	mux.HandleFunc("GET /admin-console", api.adminAppRedirect)
	mux.HandleFunc("GET /admin-console/", api.adminApp)
	mux.HandleFunc("GET /", api.publicApp)
	return requestID(mux)
}

func (api *API) publicApp(w http.ResponseWriter, r *http.Request) {
	serveStaticApp(w, r, api.publicStaticDir, r.URL.Path)
}

func (api *API) adminAppRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin-console/", http.StatusTemporaryRedirect)
}

func (api *API) adminApp(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin-console")
	serveStaticApp(w, r, api.adminStaticDir, path)
}

func (api *API) agentAppRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/agent-console/", http.StatusTemporaryRedirect)
}

func (api *API) agentApp(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/agent-console")
	serveStaticApp(w, r, api.agentStaticDir, path)
}

func serveStaticApp(w http.ResponseWriter, r *http.Request, staticDir, requestPath string) {
	if staticDir == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Clean(requestPath)
	if path == "/" {
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
		return
	}
	file := filepath.Join(staticDir, strings.TrimPrefix(path, "/"))
	if _, err := os.Stat(file); err != nil {
		if filepath.Ext(path) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
		return
	}
	http.ServeFile(w, r, file)
}

func (api *API) health(w http.ResponseWriter, r *http.Request) {
	writeOK(w, r, map[string]any{"status": "ok"})
}

func (api *API) publicLicenseQuery(w http.ResponseWriter, r *http.Request) {
	var input service.PublicLicenseQueryInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.QueryPublicLicense(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) publicProductKeys(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.ProductKeysByAppKey(r.Context(), r.URL.Query().Get("app_key"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) adminLogin(w http.ResponseWriter, r *http.Request) {
	var input service.AdminLoginInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.AdminLogin(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) adminProfile(w http.ResponseWriter, r *http.Request) {
	writeOK(w, r, adminSessionFrom(r.Context()))
}

func (api *API) createAdminUser(w http.ResponseWriter, r *http.Request) {
	var input service.CreateAdminUserInput
	if !decode(w, r, &input) {
		return
	}
	user, err := api.service.CreateAdminUser(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, user)
}

func (api *API) listAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := api.service.ListAdminUsers(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, users)
}

func (api *API) changeAdminPassword(w http.ResponseWriter, r *http.Request) {
	var input service.ChangeAdminPasswordInput
	if !decode(w, r, &input) {
		return
	}
	session := adminSessionFrom(r.Context())
	if err := api.service.ChangeAdminPassword(r.Context(), session.UserID, input); err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]any{"changed": true})
}

func (api *API) createProduct(w http.ResponseWriter, r *http.Request) {
	var input service.CreateProductInput
	if !decode(w, r, &input) {
		return
	}
	product, err := api.service.CreateProduct(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, product)
}

func (api *API) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := api.service.ListProducts(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, products)
}

func (api *API) productKeys(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.ProductKeys(r.Context(), r.PathValue("productID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) rotateProductKey(w http.ResponseWriter, r *http.Request) {
	session := adminSessionFrom(r.Context())
	result, err := api.service.RotateProductKey(r.Context(), r.PathValue("productID"), session.UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) productDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/products/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	statusByAction := map[string]string{
		"enable":  domain.ProductActive,
		"disable": domain.ProductDisabled,
	}
	status, ok := statusByAction[parts[1]]
	if !ok {
		http.NotFound(w, r)
		return
	}
	session := adminSessionFrom(r.Context())
	product, err := api.service.ChangeProductStatus(r.Context(), parts[0], status, session.UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, product)
}

func (api *API) createAgent(w http.ResponseWriter, r *http.Request) {
	var input service.CreateAgentInput
	if !decode(w, r, &input) {
		return
	}
	agent, err := api.service.CreateAgent(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, agent)
}

func (api *API) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := api.service.ListAgents(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, agents)
}

func (api *API) agentDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/agents/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && r.Method == http.MethodPost {
		statusByAction := map[string]string{
			"enable":  domain.AgentActive,
			"suspend": domain.AgentSuspended,
			"disable": domain.AgentDisabled,
		}
		if status, ok := statusByAction[parts[1]]; ok {
			session := adminSessionFrom(r.Context())
			agent, err := api.service.ChangeAgentStatus(r.Context(), parts[0], status, session.UserID)
			if err != nil {
				writeError(w, r, err)
				return
			}
			writeOK(w, r, agent)
			return
		}
	}
	if len(parts) == 4 && parts[1] == "users" && r.Method == http.MethodPost {
		statusByAction := map[string]string{
			"enable":  domain.AgentUserActive,
			"disable": domain.AgentUserDisabled,
		}
		if status, ok := statusByAction[parts[3]]; ok {
			session := adminSessionFrom(r.Context())
			user, err := api.service.ChangeAgentUserStatus(r.Context(), parts[0], parts[2], status, session.UserID)
			if err != nil {
				writeError(w, r, err)
				return
			}
			writeOK(w, r, user)
			return
		}
	}
	if len(parts) == 2 && parts[1] == "policies" {
		policies, err := api.service.ListAgentProductPolicies(r.Context(), parts[0])
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, policies)
		return
	}
	if len(parts) == 2 && parts[1] == "quotas" {
		quotas, err := api.service.ListAgentQuotaSummaries(r.Context(), parts[0])
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, quotas)
		return
	}
	if len(parts) == 2 && parts[1] == "quota-ledgers" {
		ledgers, err := api.service.ListAgentQuotaLedgers(r.Context(), parts[0])
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, ledgers)
		return
	}
	if len(parts) == 2 && parts[1] == "users" {
		switch r.Method {
		case http.MethodGet:
			users, err := api.service.ListAgentUsers(r.Context(), parts[0])
			if err != nil {
				writeError(w, r, err)
				return
			}
			writeOK(w, r, users)
			return
		case http.MethodPost:
			var input service.CreateAgentUserInput
			if !decode(w, r, &input) {
				return
			}
			input.AgentID = parts[0]
			user, err := api.service.CreateAgentUser(r.Context(), input)
			if err != nil {
				writeError(w, r, err)
				return
			}
			writeOK(w, r, user)
			return
		}
	}
	if len(parts) == 2 && parts[1] == "card-batches" && r.Method == http.MethodPost {
		var input service.AgentCreateCardBatchInput
		if !decode(w, r, &input) {
			return
		}
		input.AgentID = parts[0]
		result, err := api.service.AgentCreateCardBatch(r.Context(), input)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, result)
		return
	}
	http.NotFound(w, r)
}

func (api *API) upsertAgentPolicy(w http.ResponseWriter, r *http.Request) {
	var input service.AgentProductPolicyInput
	if !decode(w, r, &input) {
		return
	}
	policy, err := api.service.UpsertAgentProductPolicy(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, policy)
}

func (api *API) grantAgentQuota(w http.ResponseWriter, r *http.Request) {
	var input service.AgentQuotaInput
	if !decode(w, r, &input) {
		return
	}
	ledger, err := api.service.GrantAgentQuota(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, ledger)
}

func (api *API) createCardBatch(w http.ResponseWriter, r *http.Request) {
	var input service.CreateCardBatchInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.CreateCardBatch(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) listCardBatches(w http.ResponseWriter, r *http.Request) {
	batches, err := api.service.ListCardBatches(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, batches)
}

func (api *API) cardBatchDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/card-batches/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "cards" && r.Method == http.MethodGet {
		cards, err := api.service.ListCardsByBatch(r.Context(), parts[0])
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, cards)
		return
	}
	if len(parts) == 2 && parts[1] == "export" && r.Method == http.MethodPost {
		session := adminSessionFrom(r.Context())
		result, err := api.service.ExportCardBatch(r.Context(), parts[0], session.UserID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, result)
		return
	}
	http.NotFound(w, r)
}

func (api *API) voidCard(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CardID string `json:"card_id"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &input) {
		return
	}
	session := adminSessionFrom(r.Context())
	card, err := api.service.VoidCard(r.Context(), input.CardID, input.Reason, session.UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, card)
}

func (api *API) listLicenses(w http.ResponseWriter, r *http.Request) {
	page, err := queryInt(r, "page", 1)
	if err != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	pageSize, err := queryInt(r, "page_size", 20)
	if err != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	licenses, err := api.service.ListLicensesPage(r.Context(), service.LicenseListFilter{
		Status:    r.URL.Query().Get("status"),
		ProductID: r.URL.Query().Get("product_id"),
		AgentID:   r.URL.Query().Get("agent_id"),
		Query:     r.URL.Query().Get("q"),
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, licenses)
}

func queryInt(r *http.Request, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, service.ErrInvalidRequest
	}
	return value, nil
}

func (api *API) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, r, service.ErrInvalidRequest)
			return
		}
		limit = parsed
	}
	logs, err := api.service.ListAuditLogs(r.Context(), service.AuditLogFilter{
		ActorType: r.URL.Query().Get("actor_type"),
		Result:    r.URL.Query().Get("result"),
		Action:    r.URL.Query().Get("action"),
		Limit:     limit,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, logs)
}

func (api *API) createOfflineLicense(w http.ResponseWriter, r *http.Request) {
	var input service.CreateOfflineLicenseInput
	if !decode(w, r, &input) {
		return
	}
	session := adminSessionFrom(r.Context())
	license, err := api.service.CreateOfflineLicense(r.Context(), input, session.UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, license)
}

func (api *API) listOfflineLicenses(w http.ResponseWriter, r *http.Request) {
	licenses, err := api.service.ListOfflineLicenses(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, licenses)
}

func (api *API) offlineLicenseDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/offline-licenses/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	session := adminSessionFrom(r.Context())
	switch {
	case r.Method == http.MethodGet && parts[1] == "download":
		download, err := api.service.DownloadOfflineLicense(r.Context(), parts[0], session.UserID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.Filename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(download.Content)
	case r.Method == http.MethodPost && parts[1] == "revoke":
		var input struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &input) {
			return
		}
		license, err := api.service.RevokeOfflineLicense(r.Context(), parts[0], input.Reason, session.UserID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, license)
	default:
		http.NotFound(w, r)
	}
}

func (api *API) licenseDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/licenses/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "bindings" {
		bindings, err := api.service.ListLicenseBindings(r.Context(), parts[0])
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, bindings)
		return
	}
	http.NotFound(w, r)
}

func (api *API) activate(w http.ResponseWriter, r *http.Request) {
	var input service.ActivateInput
	if !decode(w, r, &input) {
		return
	}
	input.ClientIP = api.clientIP(r)
	input.UserAgent = r.UserAgent()
	result, err := api.service.Activate(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) verify(w http.ResponseWriter, r *http.Request) {
	var input service.VerifyInput
	if !decode(w, r, &input) {
		return
	}
	input.ClientIP = api.clientIP(r)
	input.UserAgent = r.UserAgent()
	result, err := api.service.Verify(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) heartbeat(w http.ResponseWriter, r *http.Request) {
	var input service.VerifyInput
	if !decode(w, r, &input) {
		return
	}
	input.ClientIP = api.clientIP(r)
	input.UserAgent = r.UserAgent()
	result, err := api.service.Heartbeat(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) renew(w http.ResponseWriter, r *http.Request) {
	var input service.RenewInput
	if !decode(w, r, &input) {
		return
	}
	input.ClientIP = api.clientIP(r)
	input.UserAgent = r.UserAgent()
	result, err := api.service.Renew(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) unbind(w http.ResponseWriter, r *http.Request) {
	var input service.UnbindInput
	if !decode(w, r, &input) {
		return
	}
	input.ClientIP = api.clientIP(r)
	input.UserAgent = r.UserAgent()
	result, err := api.service.Unbind(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) revokeLicense(w http.ResponseWriter, r *http.Request) {
	var input service.RevokeLicenseInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.RevokeLicense(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) adminUnbind(w http.ResponseWriter, r *http.Request) {
	var input service.AdminUnbindInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.AdminUnbind(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) agentLogin(w http.ResponseWriter, r *http.Request) {
	var input service.AgentLoginInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.AgentLogin(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) agentProfile(w http.ResponseWriter, r *http.Request) {
	session, err := api.agentSession(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, session)
}

func (api *API) agentProducts(w http.ResponseWriter, r *http.Request) {
	session, err := api.agentSession(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	products, err := api.service.ListAgentProducts(r.Context(), session.AgentID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, products)
}

func (api *API) agentQuotas(w http.ResponseWriter, r *http.Request) {
	session, err := api.agentSession(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	quotas, err := api.service.ListAgentQuotaSummaries(r.Context(), session.AgentID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, quotas)
}

func (api *API) agentQuotaLedgers(w http.ResponseWriter, r *http.Request) {
	session, err := api.agentSession(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	ledgers, err := api.service.ListAgentQuotaLedgers(r.Context(), session.AgentID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, ledgers)
}

func (api *API) agentCardBatches(w http.ResponseWriter, r *http.Request) {
	session, err := api.agentSession(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	batches, err := api.service.ListAgentCardBatches(r.Context(), session.AgentID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, batches)
}

func (api *API) agentCardBatchDetail(w http.ResponseWriter, r *http.Request) {
	session, err := api.agentSession(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/agent/card-batches/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "cards":
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		cards, err := api.service.ListAgentCardsByBatch(r.Context(), session.AgentID, parts[0])
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, cards)
	case "export":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if session.Role != domain.AgentRoleOwner && session.Role != domain.AgentRoleManager {
			writeError(w, r, service.ErrPermissionDenied)
			return
		}
		result, err := api.service.ExportAgentCardBatch(r.Context(), session.AgentID, session.UserID, parts[0])
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, result)
	default:
		http.NotFound(w, r)
	}
}

func (api *API) agentLicenses(w http.ResponseWriter, r *http.Request) {
	session, err := api.agentSession(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	licenses, err := api.service.ListAgentLicenses(r.Context(), session.AgentID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, licenses)
}

func (api *API) agentCreateCardBatch(w http.ResponseWriter, r *http.Request) {
	var input service.AgentCreateCardBatchInput
	if !decode(w, r, &input) {
		return
	}
	session, err := api.agentSession(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if session.Role != domain.AgentRoleOwner && session.Role != domain.AgentRoleManager {
		writeError(w, r, service.ErrPermissionDenied)
		return
	}
	input.AgentID = session.AgentID
	result, err := api.service.AgentCreateCardBatch(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) agentSession(r *http.Request) (service.AgentSession, error) {
	if token := bearerToken(r); token != "" {
		return api.service.AuthenticateAgentToken(r.Context(), token)
	}
	return service.AgentSession{}, service.ErrInvalidAgentToken
}

type adminSessionContextKey struct{}

func (api *API) withAdmin(next http.HandlerFunc, allowedRoles ...string) http.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, role := range allowedRoles {
		allowed[role] = struct{}{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeError(w, r, service.ErrInvalidAdminToken)
			return
		}
		session, err := api.service.AuthenticateAdminToken(r.Context(), token)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if _, ok := allowed[session.Role]; !ok {
			writeError(w, r, service.ErrPermissionDenied)
			return
		}
		ctx := context.WithValue(r.Context(), adminSessionContextKey{}, session)
		next(w, r.WithContext(ctx))
	}
}

func adminSessionFrom(ctx context.Context) service.AdminSession {
	session, _ := ctx.Value(adminSessionContextKey{}).(service.AdminSession)
	return session
}

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ""
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return false
	}
	return true
}

func writeOK(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"request_id": requestIDFrom(r),
		"data":       data,
	})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := "internal server error"
	var appErr service.AppError
	if errors.As(err, &appErr) {
		status = appErr.HTTPStatus
		code = appErr.Code
		message = appErr.Message
	}
	writeJSON(w, status, map[string]any{
		"success":    false,
		"request_id": requestIDFrom(r),
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (api *API) clientIP(r *http.Request) string {
	if api.trustProxyHeaders {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			if ip := net.ParseIP(strings.TrimSpace(strings.Split(forwarded, ",")[0])); ip != nil {
				return ip.String()
			}
		}
		if realIP := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); realIP != nil {
			return realIP.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
}

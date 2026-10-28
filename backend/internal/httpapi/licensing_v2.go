package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func (api *API) v2Principal(next http.HandlerFunc) http.HandlerFunc {
	roles := append(append([]string{}, adminReadRoles...), domain.AdminRoleDeveloper)
	return api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		session := adminSessionFrom(r.Context())
		kind := "admin"
		if session.Role == domain.AdminRoleDeveloper {
			kind = "developer"
		}
		if kind == "admin" && len(api.adminNetworks) > 0 {
			address, err := netip.ParseAddr(api.clientIP(r))
			if err != nil || !containsIP(api.adminNetworks, address) {
				writeError(w, r, service.ErrPermissionDenied)
				return
			}
		}
		ctx := core.BindPrincipal(r.Context(), core.Principal{Actor: core.Actor{Kind: kind, ID: session.UserID, Role: session.Role, RequestID: requestIDFrom(r)}})
		next(w, r.WithContext(ctx))
	}, roles...)
}
func (api *API) v2Admin(next http.HandlerFunc, write bool) http.HandlerFunc {
	return api.v2Principal(func(w http.ResponseWriter, r *http.Request) {
		p, e := core.PrincipalFrom(r.Context())
		if e != nil {
			writeError(w, r, e)
			return
		}
		if p.Actor.Kind != "admin" || write && p.Actor.Role != "admin" && p.Actor.Role != "super_admin" {
			writeError(w, r, service.ErrPermissionDenied)
			return
		}
		next(w, r)
	})
}
func (api *API) v2Customer(next http.HandlerFunc) http.HandlerFunc {
	return api.withCustomer(func(w http.ResponseWriter, r *http.Request) {
		customer := customerFrom(r.Context())
		ctx := core.BindPrincipal(r.Context(), core.Principal{Actor: core.Actor{Kind: "customer", ID: customer.ID, Role: "customer", RequestID: requestIDFrom(r)}})
		next(w, r.WithContext(ctx))
	})
}
func v2Response(w http.ResponseWriter, r *http.Request, data any, e error) {
	w.Header().Set("Cache-Control", "no-store")
	if e != nil {
		writeError(w, r, e)
		return
	}
	writeOK(w, r, data)
}
func (api *API) registerLicensingV2(mux *http.ServeMux) {
	notFoundV2 := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, core.Error{Code: "API_NOT_FOUND", Message: "接口不存在，请查看当前应用的接入文档", Status: 404})
	}
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		mux.HandleFunc(method+" /v2/", notFoundV2)
	}
	mux.HandleFunc("GET /v2/public/applications", withRateLimit(api.publicQueryLimit, func(w http.ResponseWriter, r *http.Request) {
		data, e := api.service.Licensing().PublicApplications(r.Context())
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("POST /v2/public/query", withRateLimit(api.publicQueryLimit, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			core.PublicLookupInput
			CaptchaID   string `json:"captcha_id"`
			CaptchaCode string `json:"captcha_code"`
		}
		if !decode(w, r, &in) {
			return
		}
		free, _ := api.publicQueryFreeLimit.Allow("v2/public/" + api.clientIP(r))
		if !free || in.CaptchaID != "" || in.CaptchaCode != "" {
			if e := api.service.EnsureCaptcha(r.Context(), in.CaptchaID, in.CaptchaCode); e != nil {
				writeError(w, r, e)
				return
			}
		}
		data, e := api.service.Licensing().PublicLookup(r.Context(), in.PublicLookupInput)
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("POST /v2/platform/group-agent-grants", api.v2Principal(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Group string `json:"group"`
			core.AgentGrant
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := api.service.Licensing().SaveGroupAgentGrants(r.Context(), in.Group, in.AgentGrant)
		v2Response(w, r, v, e)
	}))
	mux.HandleFunc("GET /v2/platform/agents/{agentID}/records", api.v2Admin(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.Licensing().AdminAgentHistory(r.Context(), r.PathValue("agentID"))
		v2Response(w, r, v, e)
	}, false))
	mux.HandleFunc("GET /v2/platform/authorization-archive", api.v2Admin(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.AuthorizationArchive(r.Context())
		v2Response(w, r, v, e)
	}, false))
	mux.HandleFunc("GET /v2/platform/developers", api.v2Admin(func(w http.ResponseWriter, r *http.Request) {
		v, e := api.service.LicensingDevelopers(r.Context())
		v2Response(w, r, v, e)
	}, true))
	mux.HandleFunc("GET /v2/console/applications", api.v2Principal(api.v2Applications))
	mux.HandleFunc("POST /v2/console/applications", api.v2Principal(api.v2Applications))
	mux.HandleFunc("GET /v2/console/applications/{appID}", api.v2Principal(api.v2Application))
	mux.HandleFunc("POST /v2/console/applications/{appID}", api.v2Principal(api.v2Application))
	mux.HandleFunc("GET /v2/console/applications/{appID}/{collection}", api.v2Principal(api.v2Collection))
	mux.HandleFunc("POST /v2/console/applications/{appID}/{collection}", api.v2Principal(api.v2Collection))
	mux.HandleFunc("POST /v2/console/applications/{appID}/{collection}/{itemID}/{action}", api.v2Principal(api.v2ItemAction))
	mux.HandleFunc("GET /v2/applications/{appID}/.well-known/jwks.json", api.v2JWKS)
	mux.HandleFunc("GET /.well-known/jwks.json", api.v2JWKS)
	mux.HandleFunc("POST /v2/server/applications/{appID}/orders", api.v2ServerPrincipal(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			core.SaleInput
			Confirmed bool `json:"confirmed"`
		}
		if !decode(w, r, &in) {
			return
		}
		data, e := api.service.Licensing().CreateSale(r.Context(), r.PathValue("appID"), in.SaleInput, in.Confirmed)
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("GET /v2/server/applications/{appID}/licenses", api.v2ServerPrincipal(func(w http.ResponseWriter, r *http.Request) {
		data, e := api.service.Licensing().ListLicenses(r.Context(), r.PathValue("appID"))
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("POST /v2/console/applications/{appID}/releases/upload", api.v2Principal(api.v2UploadRelease))
	mux.HandleFunc("POST /v2/updates/check", api.v2CheckUpdate)
	mux.HandleFunc("POST /v2/updates/{releaseID}/authorize", api.v2AuthorizeUpdate)
	mux.HandleFunc("POST /v2/updates/{releaseID}/download", api.v2DownloadUpdate)
	mux.HandleFunc("POST /v2/security/report", api.v2IntegrityReport)
	mux.HandleFunc("GET /v2/applications/{appID}/block-page", api.v2BlockPage)
	mux.HandleFunc("POST /v2/activate", api.v2Activate)
	mux.HandleFunc("POST /v2/verify", api.v2Verify)
	mux.HandleFunc("POST /v2/installations/request", api.v2InstallationRequest)
	mux.HandleFunc("POST /v2/sales/{appID}/{channelID}", api.v2Webhook)
	mux.HandleFunc("GET /v2/agent/records", api.v2AgentPrincipal(func(w http.ResponseWriter, r *http.Request) {
		data, e := api.service.Licensing().AgentHistory(r.Context())
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("GET /v2/agent/catalog", api.v2AgentPrincipal(func(w http.ResponseWriter, r *http.Request) {
		data, e := api.service.Licensing().AgentCatalog(r.Context())
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("GET /v2/agent/applications/{appID}/orders", api.v2AgentPrincipal(func(w http.ResponseWriter, r *http.Request) {
		data, e := api.service.Licensing().AgentOrders(r.Context(), r.PathValue("appID"))
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("POST /v2/agent/applications/{appID}/orders", api.v2AgentPrincipal(func(w http.ResponseWriter, r *http.Request) {
		var in core.AgentSaleInput
		if !decode(w, r, &in) {
			return
		}
		data, e := api.service.Licensing().AgentSale(r.Context(), r.PathValue("appID"), in)
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("GET /v2/user/entitlements", api.v2Customer(func(w http.ResponseWriter, r *http.Request) {
		data, e := api.service.Licensing().CustomerEntitlements(r.Context())
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("GET /v2/user/licenses", api.v2Customer(func(w http.ResponseWriter, r *http.Request) {
		data, e := api.service.Licensing().CustomerLicenses(r.Context())
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("POST /v2/user/applications/{appID}/claim", api.v2Customer(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			CardCode string `json:"card_code"`
		}
		if !decode(w, r, &in) {
			return
		}
		data, e := api.service.Licensing().ClaimCustomerLicense(r.Context(), r.PathValue("appID"), in.CardCode)
		v2Response(w, r, data, e)
	}))
	mux.HandleFunc("POST /v2/user/applications/{appID}/licenses/{itemID}/{action}", withRateLimit(api.customerRebindLimit, api.v2Customer(api.v2CustomerAction)))
}
func (api *API) v2Applications(w http.ResponseWriter, r *http.Request) {
	s := api.service.Licensing()
	if r.Method == "GET" {
		data, e := s.ListApplications(r.Context())
		v2Response(w, r, data, e)
		return
	}
	var in core.CreateApplicationInput
	if !decode(w, r, &in) {
		return
	}
	if adminSessionFrom(r.Context()).Role != domain.AdminRoleDeveloper {
		if e := api.service.ValidateLicensingDeveloper(r.Context(), in.DeveloperID); e != nil {
			writeError(w, r, e)
			return
		}
	}
	data, e := s.CreateApplication(r.Context(), in)
	v2Response(w, r, data, e)
}
func (api *API) v2Application(w http.ResponseWriter, r *http.Request) {
	s := api.service.Licensing()
	id := r.PathValue("appID")
	if r.Method == "GET" {
		data, e := s.GetApplication(r.Context(), id)
		v2Response(w, r, data, e)
		return
	}
	var in struct {
		core.CreateApplicationInput
		Status string `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	data, e := s.SaveApplication(r.Context(), id, in.CreateApplicationInput, in.Status)
	v2Response(w, r, data, e)
}
func (api *API) v2Collection(w http.ResponseWriter, r *http.Request) {
	s := api.service.Licensing()
	ctx := r.Context()
	id := r.PathValue("appID")
	collection := r.PathValue("collection")
	var data any
	var e error
	if r.Method == "GET" {
		switch collection {
		case "integration":
			data, e = s.Integration(ctx, id, r.URL.Query().Get("language"), core.IntegrationOptions{PlanID: r.URL.Query().Get("plan_id"), BindingType: r.URL.Query().Get("binding_type"), Form: r.URL.Query().Get("form"), BaseURL: r.URL.Query().Get("base_url")})
		case "integration.zip":
			kit, err := s.Integration(ctx, id, r.URL.Query().Get("language"), core.IntegrationOptions{PlanID: r.URL.Query().Get("plan_id"), BindingType: r.URL.Query().Get("binding_type"), Form: r.URL.Query().Get("form"), BaseURL: r.URL.Query().Get("base_url")})
			if err != nil {
				writeError(w, r, err)
				return
			}
			raw, err := core.IntegrationZIP(kit)
			if err != nil {
				writeError(w, r, err)
				return
			}
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": id + "-" + kit.Language + "-integration.zip"}))
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(raw)
			return
		case "releases":
			data, e = s.ListReleases(ctx, id)
		case "agent-grants":
			data, e = s.ListAgentGrants(ctx, id)
		case "plans":
			data, e = s.ListPlans(ctx, id)
		case "licenses":
			data, e = s.ListLicenses(ctx, id)
		case "activations":
			data, e = s.ListActivations(ctx, id, r.URL.Query().Get("license_id"))
		case "cards":
			data, e = s.ListCards(ctx, id)
		case "orders":
			data, e = s.ListSales(ctx, id)
		case "binding-rules":
			data, e = s.ListBindingRules(ctx, id)
		case "pending-installations":
			data, e = s.ListPendingInstallations(ctx, id)
		case "sales-channels":
			data, e = s.ListSalesChannels(ctx, id)
		case "diagnostics":
			data, e = s.Diagnostics(ctx, id)
		case "audit":
			data, e = s.AuditEvents(ctx, id, core.Filter{EventType: r.URL.Query().Get("event_type"), Result: r.URL.Query().Get("result"), ReasonCode: r.URL.Query().Get("reason_code"), Limit: 500, Offset: v2AuditOffset(r)})
		case "audit-integrity":
			data, e = s.AuditIntegrity(ctx, id)
		case "jwks":
			if _, e = s.GetApplication(ctx, id); e == nil {
				data, e = s.PublicKeys(ctx, id)
			}
		default:
			http.NotFound(w, r)
			return
		}
		v2Response(w, r, data, e)
		return
	}
	switch collection {
	case "agent-grants":
		var in core.AgentGrant
		if !decode(w, r, &in) {
			return
		}
		data, e = s.SaveAgentGrant(ctx, id, in)
	case "plans":
		var in core.PlanInput
		if !decode(w, r, &in) {
			return
		}
		data, e = s.SavePlan(ctx, id, "", in)
	case "orders":
		var in struct {
			core.SaleInput
			Confirmed bool `json:"confirmed"`
		}
		if !decode(w, r, &in) {
			return
		}
		data, e = s.CreateSale(ctx, id, in.SaleInput, in.Confirmed)
	case "binding-rules":
		var in core.BindingRule
		if !decode(w, r, &in) {
			return
		}
		data, e = s.SaveBindingRule(ctx, id, "", in)
	case "sales-channels":
		var in core.ChannelInput
		if !decode(w, r, &in) {
			return
		}
		data, e = s.SaveSalesChannel(ctx, id, "", in, false)
	case "rotate-key":
		var in struct {
			Reason            string `json:"reason"`
			RetireImmediately bool   `json:"retire_immediately"`
		}
		if !decode(w, r, &in) {
			return
		}
		data, e = s.RotateSigningKey(ctx, id, in.Reason, in.RetireImmediately)
	case "assign-developer":
		var in struct {
			DeveloperID string `json:"developer_id"`
			Reason      string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		if e = api.service.ValidateLicensingDeveloper(ctx, in.DeveloperID); e == nil {
			data, e = s.AssignDeveloper(ctx, id, in.DeveloperID, in.Reason)
		}
	case "reset-secret":
		var in struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		data, e = s.ResetAppSecret(ctx, id, in.Reason)
	default:
		http.NotFound(w, r)
		return
	}
	v2Response(w, r, data, e)
}
func (api *API) v2ItemAction(w http.ResponseWriter, r *http.Request) {
	s := api.service.Licensing()
	ctx := r.Context()
	id, item, action := r.PathValue("appID"), r.PathValue("itemID"), r.PathValue("action")
	var data any = map[string]bool{"updated": true}
	var e error
	switch r.PathValue("collection") {
	case "releases":
		var in struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}
		if action != "status" {
			http.NotFound(w, r)
			return
		}
		if !decode(w, r, &in) {
			return
		}
		data, e = s.SetReleaseStatus(ctx, id, item, in.Status, in.Reason)
	case "plans":
		if action != "save" {
			http.NotFound(w, r)
			return
		}
		var in core.PlanInput
		if !decode(w, r, &in) {
			return
		}
		data, e = s.SavePlan(ctx, id, item, in)
	case "orders":
		if action != "confirm" && action != "refund" {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		if action == "refund" {
			data, e = s.RefundSale(ctx, id, item, in.Reason)
		} else {
			data, e = s.ConfirmSale(ctx, id, item, in.Reason)
		}
	case "cards":
		if action != "revoke" {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		e = s.RevokeCard(ctx, id, item, in.Reason)
	case "activations":
		if action != "deactivate" {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		e = s.DeactivateInstallation(ctx, id, item, in.Reason)
	case "binding-rules":
		if action == "save" {
			var in core.BindingRule
			if !decode(w, r, &in) {
				return
			}
			data, e = s.SaveBindingRule(ctx, id, item, in)
		} else if action == "remove" {
			var in struct {
				Reason string `json:"reason"`
			}
			if !decode(w, r, &in) {
				return
			}
			e = s.RemoveBindingRule(ctx, id, item, in.Reason)
		} else {
			http.NotFound(w, r)
			return
		}
	case "sales-channels":
		if action != "save" && action != "rotate" {
			http.NotFound(w, r)
			return
		}
		var in core.ChannelInput
		if !decode(w, r, &in) {
			return
		}
		data, e = s.SaveSalesChannel(ctx, id, item, in, action == "rotate")
	case "pending-installations":
		var in struct {
			LicenseID string `json:"license_id"`
			Reason    string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		e = s.ApproveInstallation(ctx, id, item, in.LicenseID, action, in.Reason)
	case "licenses":
		switch action {
		case "security":
			var in core.SecurityDecisionInput
			if !decode(w, r, &in) {
				return
			}
			e = s.ReviewLicenseSecurity(ctx, id, item, in)
		case "status":
			var in struct {
				Status string `json:"status"`
				Reason string `json:"reason"`
			}
			if !decode(w, r, &in) {
				return
			}
			e = s.SetLicenseStatus(ctx, id, item, in.Status, in.Reason)
		case "binding", "rebind":
			var in struct {
				Binding core.BindingInput `json:"binding"`
				Reason  string            `json:"reason"`
			}
			if !decode(w, r, &in) {
				return
			}
			if action == "binding" {
				e = s.RegisterBinding(ctx, id, item, in.Binding, in.Reason)
			} else {
				e = s.Rebind(ctx, id, item, in.Binding, in.Reason)
			}
		case "invite":
			var in struct {
				Hours int `json:"hours"`
			}
			if !decode(w, r, &in) {
				return
			}
			data, e = s.CreateInvitation(ctx, id, item, in.Hours)
		case "owner":
			var in struct {
				UserID string `json:"user_id"`
				Reason string `json:"reason"`
			}
			if !decode(w, r, &in) {
				return
			}
			e = s.AssignLicenseOwner(ctx, id, item, in.UserID, in.Reason)
		default:
			http.NotFound(w, r)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	v2Response(w, r, data, e)
}
func (api *API) v2CustomerAction(w http.ResponseWriter, r *http.Request) {
	s := api.service.Licensing()
	id, item := r.PathValue("appID"), r.PathValue("itemID")
	var data any = map[string]bool{"updated": true}
	var e error
	switch r.PathValue("action") {
	case "appeal":
		var in struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		data, e = s.SubmitSecurityAppeal(r.Context(), id, item, in.Reason)
	case "invite":
		var in struct {
			Hours int `json:"hours"`
		}
		if !decode(w, r, &in) {
			return
		}
		data, e = s.CreateInvitation(r.Context(), id, item, in.Hours)
	case "deactivate":
		var in struct {
			ActivationID string `json:"activation_id"`
			Reason       string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		e = s.CustomerDeactivate(r.Context(), id, item, in.ActivationID, in.Reason)
	case "renew":
		var in struct {
			CardCode string `json:"card_code"`
		}
		if !decode(w, r, &in) {
			return
		}
		data, e = s.CustomerRenew(r.Context(), id, item, in.CardCode)
	case "rebind":
		var in struct {
			Binding core.BindingInput `json:"binding"`
			Reason  string            `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		e = s.Rebind(r.Context(), id, item, in.Binding, in.Reason)
	default:
		http.NotFound(w, r)
		return
	}
	v2Response(w, r, data, e)
}
func (api *API) v2JWKS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("appID")
	if id == "" {
		id = r.URL.Query().Get("app_id")
	}
	keys, e := api.service.Licensing().PublicKeys(r.Context(), id)
	if e != nil {
		writeError(w, r, e)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, 200, keys)
}
func (api *API) v2SignedBody(w http.ResponseWriter, r *http.Request, target any) (Signed core.SignedRequest, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		writeError(w, r, core.Error{Code: "REQUEST_TOO_LARGE", Message: "请求最大 64 KiB", Status: 413})
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	stamp, e := strconv.ParseInt(r.Header.Get("X-YN-Timestamp"), 10, 64)
	if e != nil {
		writeError(w, r, core.Error{Code: "REQUEST_TIMESTAMP_INVALID", Message: "缺少签名时间戳", Status: 403})
		return
	}
	Signed = core.SignedRequest{Method: r.Method, Path: r.URL.EscapedPath(), RemoteIP: api.clientIP(r), RequestID: requestIDFrom(r), Body: raw, Proof: core.RequestProof{Timestamp: stamp, Nonce: r.Header.Get("X-YN-Nonce"), Signature: r.Header.Get("X-YN-Signature")}}
	return Signed, true
}
func (api *API) v2Rate(w http.ResponseWriter, r *http.Request, app string, write bool) bool {
	limiter := api.clientReadLimit
	if write {
		limiter = api.clientWriteLimit
	}
	route := r.Pattern
	if route == "" {
		route = r.URL.Path
	}
	key := "v2/" + app + "/" + api.clientIP(r) + "/" + route
	allowed, retry := limiter.Allow("v2/ip/" + api.clientIP(r))
	if allowed {
		allowed, retry = limiter.Allow(key)
	}
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry/time.Second)+1))
		writeError(w, r, service.AppError{Code: "RATE_LIMITED", Message: "请求过于频繁，请稍后重试", HTTPStatus: 429})
	}
	return allowed
}
func (api *API) v2Activate(w http.ResponseWriter, r *http.Request) {
	var in core.ActivateInput
	proof, ok := api.v2SignedBody(w, r, &in)
	if !ok || !api.v2Rate(w, r, in.AppID, true) {
		return
	}
	data, e := api.service.Licensing().Activate(r.Context(), in, proof)
	v2Response(w, r, data, e)
}
func (api *API) v2Verify(w http.ResponseWriter, r *http.Request) {
	var in core.VerifyInput
	proof, ok := api.v2SignedBody(w, r, &in)
	if !ok || !api.v2Rate(w, r, in.AppID, false) {
		return
	}
	data, e := api.service.Licensing().Verify(r.Context(), in, proof)
	v2Response(w, r, data, e)
}
func (api *API) v2InstallationRequest(w http.ResponseWriter, r *http.Request) {
	var in core.PendingInput
	proof, ok := api.v2SignedBody(w, r, &in)
	if !ok || !api.v2Rate(w, r, in.AppID, true) {
		return
	}
	data, e := api.service.Licensing().RequestInstallation(r.Context(), in, proof)
	v2Response(w, r, data, e)
}
func (api *API) v2Webhook(w http.ResponseWriter, r *http.Request) {
	if !api.v2Rate(w, r, r.PathValue("appID"), true) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	data, e := api.service.Licensing().ReceiveSale(r.Context(), r.PathValue("appID"), r.PathValue("channelID"), r.Header.Get("X-YN-Timestamp"), r.Header.Get("X-YN-Signature"), raw)
	v2Response(w, r, data, e)
}

// Translate domain errors without exposing storage or cryptographic internals.
func licensingHTTPError(e error) (int, string, string, bool) {
	var ce core.Error
	if errors.As(e, &ce) {
		return ce.Status, ce.Code, ce.Message, true
	}
	var te *core.TokenError
	if errors.As(e, &te) {
		return 403, te.Code, "安装签名或授权令牌验证失败，请检查接入配置", true
	}
	if errors.Is(e, core.ErrNotFound) {
		return 404, "NOT_FOUND", "当前应用中不存在该记录", true
	}
	if errors.Is(e, core.ErrConflict) {
		return 409, "CONFLICT", "操作冲突，请刷新重试", true
	}
	return 0, "", "", false
}

// Basic authentication is only for the developer's own trusted service. It is
// intentionally absent from customer-installation endpoints and examples.
func (api *API) v2ServerPrincipal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !api.v2Rate(w, r, r.PathValue("appID"), true) {
			return
		}
		id, secret, ok := r.BasicAuth()
		if !ok || id != r.PathValue("appID") {
			writeError(w, r, core.Error{Code: "APPLICATION_CREDENTIALS_INVALID", Message: "应用凭证无效", Status: 401})
			return
		}
		app, e := api.service.Licensing().AuthenticateAppSecret(r.Context(), id, secret)
		if e != nil {
			writeError(w, r, e)
			return
		}
		ctx := core.BindPrincipal(r.Context(), core.Principal{Actor: core.Actor{Kind: "application_server", ID: app.ApplicationID, Role: "license_issuer", RequestID: requestIDFrom(r)}})
		next(w, r.WithContext(ctx))
	}
}

func (api *API) v2AgentPrincipal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, e := api.agentSession(r)
		if e != nil {
			writeError(w, r, e)
			return
		}
		ctx := core.BindPrincipal(r.Context(), core.Principal{Actor: core.Actor{Kind: "agent", ID: session.UserID, Role: session.Role, RequestID: requestIDFrom(r)}, AgentID: session.AgentID})
		next(w, r.WithContext(ctx))
	}
}

func (api *API) v2UploadRelease(w http.ResponseWriter, r *http.Request) {
	if e := api.service.Licensing().CheckApplicationAccess(r.Context(), r.PathValue("appID"), true); e != nil {
		writeError(w, r, e)
		return
	}
	releaseSlot, e := api.service.BeginReleaseUpload(r.Context())
	if e != nil {
		writeError(w, r, e)
		return
	}
	defer releaseSlot()
	r.Body = http.MaxBytesReader(w, r.Body, core.MaxPackageSize+(1<<20))
	if e = r.ParseMultipartForm(2 << 20); e != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, e := r.FormFile("file")
	if e != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	defer file.Close()
	var target core.ReleaseTarget
	if e = json.Unmarshal([]byte(r.FormValue("target")), &target); e != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	data, e := api.service.Licensing().UploadRelease(r.Context(), r.PathValue("appID"), core.UploadInput{Version: r.FormValue("version"), Notes: r.FormValue("notes"), Filename: header.Filename, Target: target}, file)
	v2Response(w, r, data, e)
}
func (api *API) v2CheckUpdate(w http.ResponseWriter, r *http.Request) {
	var in core.UpdateInput
	proof, ok := api.v2SignedBody(w, r, &in)
	if !ok || !api.v2Rate(w, r, in.AppID, false) {
		return
	}
	data, e := api.service.Licensing().CheckUpdate(r.Context(), in, proof)
	v2Response(w, r, data, e)
}
func (api *API) v2AuthorizeUpdate(w http.ResponseWriter, r *http.Request) {
	var in core.UpdateInput
	proof, ok := api.v2SignedBody(w, r, &in)
	if !ok || !api.v2Rate(w, r, in.AppID, false) {
		return
	}
	data, e := api.service.Licensing().AuthorizeUpdate(r.Context(), in, r.PathValue("releaseID"), proof)
	v2Response(w, r, data, e)
}
func (api *API) v2DownloadUpdate(w http.ResponseWriter, r *http.Request) {
	var in core.UpdateInput
	proof, ok := api.v2SignedBody(w, r, &in)
	if !ok || !api.v2Rate(w, r, in.AppID, false) {
		return
	}
	slot, e := api.service.BeginReleaseDownload(r.Context())
	if e != nil {
		writeError(w, r, e)
		return
	}
	defer slot()
	data, file, e := api.service.Licensing().DownloadUpdate(r.Context(), in, r.PathValue("releaseID"), proof)
	if e != nil {
		writeError(w, r, e)
		return
	}
	defer file.Close()
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": data.Release.Filename}))
	w.Header().Set("Content-Length", strconv.FormatInt(data.Release.Size, 10))
	w.Header().Set("X-YN-Update-Manifest", data.ManifestJWT)
	w.Header().Set("X-Checksum-SHA256", data.Release.SHA256)
	_, _ = io.Copy(w, file)
}

func (api *API) v2IntegrityReport(w http.ResponseWriter, r *http.Request) {
	var in core.IntegrityReportInput
	proof, ok := api.v2SignedBody(w, r, &in)
	if !ok || !api.v2Rate(w, r, in.AppID, true) {
		return
	}
	data, e := api.service.Licensing().ReportIntegrity(r.Context(), in, proof)
	v2Response(w, r, data, e)
}

// Offset is validated independently from application scope; no unbounded query.
func v2AuditOffset(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

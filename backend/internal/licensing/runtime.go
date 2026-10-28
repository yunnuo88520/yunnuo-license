package licensing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// SignedRequest contains transport facts, never JSON-decoded authority. Path
// and raw body are exactly what the installation signs; RemoteIP comes from
// the server's trusted-proxy policy, never an arbitrary forwarded header.
type SignedRequest struct {
	Method, Path, RemoteIP, RequestID string
	Body                              []byte
	Proof                             RequestProof
}
type ActivateInput struct {
	AppID           string       `json:"app_id"`
	CardCode        string       `json:"card_code,omitempty"`
	InvitationID    string       `json:"invitation_id,omitempty"`
	InvitationToken string       `json:"invitation_token,omitempty"`
	PendingID       string       `json:"pending_id,omitempty"`
	PublicKey       string       `json:"public_key"`
	Binding         BindingInput `json:"binding"`
	Form            string       `json:"form"`
	SessionID       string       `json:"session_id,omitempty"`
	Label           string       `json:"label,omitempty"`
	BorrowSeconds   int          `json:"borrow_seconds,omitempty"`
}
type VerifyInput struct {
	AppID          string       `json:"app_id"`
	ActivationID   string       `json:"activation_id"`
	JWT            string       `json:"jwt,omitempty"`
	Binding        BindingInput `json:"binding"`
	Form           string       `json:"form"`
	Action         string       `json:"action,omitempty"`
	Units          int64        `json:"units,omitempty"`
	IdempotencyKey string       `json:"idempotency_key,omitempty"`
	RenewalCard    string       `json:"renewal_card,omitempty"`
}
type RuntimeResult struct {
	AppID          string     `json:"app_id"`
	LicenseID      string     `json:"license_id"`
	ActivationID   string     `json:"activation_id"`
	JWT            string     `json:"jwt"`
	OfflineJWT     string     `json:"offline_jwt,omitempty"`
	ServerTime     time.Time  `json:"server_time"`
	ExpiresAt      time.Time  `json:"expires_at"`
	RefreshAfter   time.Time  `json:"refresh_after"`
	GraceUntil     time.Time  `json:"grace_until"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at"`
	UsageExpiresAt *time.Time `json:"usage_expires_at"`
	UpdatesUntil   *time.Time `json:"updates_until"`
	RemainingUnits *int64     `json:"remaining_units,omitempty"`
	Released       bool       `json:"released"`
}

func (s *Service) authenticateInstallation(ctx context.Context, appID, public string, req SignedRequest) (context.Context, Application, Repository, error) {
	hash, e := PublicKeyFingerprint(public)
	if e != nil {
		return ctx, Application{}, nil, e
	}
	scoped, app, repo, e := s.runtimeContext(ctx, appID, Actor{Kind: "installation", ID: hash, RequestID: req.RequestID})
	if e != nil {
		return scoped, app, repo, e
	}
	if e = VerifyInstallationRequest(public, req.Method, req.Path, appID, req.Body, req.Proof, s.now()); e != nil {
		return scoped, app, repo, s.failure(scoped, repo, "request_authentication", hash, e)
	}
	// Reserve a nonce in its own committed transaction, so a later rejected
	// business operation cannot make an authenticated request replayable.
	e = repo.Update(scoped, func(tx Transaction) error {
		nonceHash := s.hash("nonce", appID, req.Proof.Nonce)
		id := s.hash("request", appID, hash+"/"+nonceHash)
		if _, e := tx.Nonces().Get(scoped, id); e == nil {
			return conflict("REQUEST_REPLAYED", "签名请求已使用，请使用新随机数重试")
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		expired, e := tx.Nonces().Find(scoped, Filter{Before: timePointer(s.now()), Limit: 100})
		if e != nil {
			return e
		}
		for _, v := range expired {
			if e = tx.Nonces().Delete(scoped, v.ID); e != nil {
				return e
			}
		}
		digest := sha256.Sum256(req.Body)
		r := RequestNonce{Record: s.record(appID, "nonce"), CredentialID: hash, NonceHash: nonceHash, BodyHash: hex.EncodeToString(digest[:]), ExpiresAt: s.now().Add(5 * time.Minute)}
		r.ID = id
		if e = tx.Nonces().Insert(scoped, r); e != nil {
			return e
		}
		return s.append(scoped, tx, "request_authenticated", hash, map[string]any{"path": req.Path})
	})
	if e != nil {
		return scoped, app, repo, s.failure(scoped, repo, "request_authentication", hash, e)
	}
	return scoped, app, repo, nil
}
func timePointer(v time.Time) *time.Time { return &v }
func (s *Service) runtimeBinding(app Application, input BindingInput, maxAliases int, req SignedRequest, publicHash string) (normalizedBinding, error) {
	if input.Type == "ip" && app.VerifyPolicy.IPSource == "egress" {
		actual, e := canonicalAddress(req.RemoteIP)
		if e != nil {
			return normalizedBinding{}, forbidden("SOURCE_IP_UNAVAILABLE", "无法确定可信来源 IP")
		}
		declared, e := canonicalAddress(input.Value)
		if e != nil || actual != declared {
			return normalizedBinding{}, forbidden("SOURCE_IP_MISMATCH", "IP 绑定必须匹配当前请求的可信出口地址")
		}
	}
	b, e := s.normalizeBinding(app.ApplicationID, input, maxAliases)
	if e != nil {
		return b, e
	}
	if input.Type == "seat" {
		b.Hash = s.hash("seat", app.ApplicationID, publicHash)
		sum := sha256.Sum256([]byte("seat\x00" + publicHash))
		b.ClientHash = hex.EncodeToString(sum[:])
	}
	return b, nil
}
func (s *Service) Activate(ctx context.Context, in ActivateInput, req SignedRequest) (RuntimeResult, error) {
	if len(in.Label) > 160 || len(in.SessionID) > 64 || !supportedForms[in.Form] {
		return RuntimeResult{}, invalid("安装标签、会话或部署形态无效")
	}
	scoped, _, repo, e := s.authenticateInstallation(ctx, in.AppID, in.PublicKey, req)
	if e != nil {
		return RuntimeResult{}, e
	}
	publicHash, _ := PublicKeyFingerprint(in.PublicKey)
	var out RuntimeResult
	e = repo.Update(scoped, func(tx Transaction) error {
		app, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		var l License
		var card *CardKey
		var invitation *Invitation
		var pending *PendingInstallation
		count := 0
		if in.CardCode != "" {
			count++
		}
		if in.InvitationID != "" {
			count++
		}
		if in.PendingID != "" {
			count++
		}
		if count != 1 {
			return invalid("请选择卡密、安装邀请或已批准的安装申请之一")
		}
		if in.CardCode != "" {
			cards, e := tx.Cards().Find(scoped, Filter{KeyHash: s.cardHash(in.AppID, in.CardCode), Limit: 1})
			if e != nil {
				return e
			}
			if len(cards) != 1 {
				return forbidden("CARD_INVALID", "卡密不属于此应用或不存在")
			}
			c := cards[0]
			if c.Status == "revoked" {
				return forbidden("CARD_REVOKED", "卡密已吊销")
			}
			if c.Purpose != "activate" {
				return invalid("此卡密为续费卡，请在已激活授权中使用")
			}
			if c.RedeemUntil != nil && !s.now().Before(*c.RedeemUntil) {
				return forbidden("CARD_EXPIRED", "卡密已超过兑换期限")
			}
			card = &c
			l, e = tx.Licenses().Get(scoped, c.BoundLicenseID)
			if e != nil {
				return e
			}
		} else if in.InvitationID != "" {
			v, e := tx.Invitations().Get(scoped, in.InvitationID)
			if e != nil {
				return forbidden("INVITATION_INVALID", "安装邀请无效")
			}
			want := s.hash("invitation", in.AppID, in.InvitationToken)
			if !constantEqual(want, v.TokenHash) || !s.now().Before(v.ExpiresAt) || v.UsedAt != nil && v.PublicKeyHash != publicHash {
				return forbidden("INVITATION_INVALID", "安装邀请已过期、已使用或不匹配")
			}
			invitation = &v
			l, e = tx.Licenses().Get(scoped, v.LicenseID)
			if e != nil {
				return e
			}
		} else {
			v, e := tx.PendingInstallations().Get(scoped, in.PendingID)
			if e != nil || v.PublicKeyHash != publicHash || v.PublicKey != in.PublicKey || v.Status != "approved" || !s.now().Before(v.ExpiresAt) {
				return forbidden("INSTALLATION_NOT_APPROVED", "安装申请尚未批准或已过期")
			}
			pending = &v
			l, e = tx.Licenses().Get(scoped, v.LicenseID)
			if e != nil {
				return e
			}
		}
		if invitation != nil && invitation.OwnerUserID != "" && invitation.OwnerUserID != l.OwnerUserID {
			return forbidden("INVITATION_INVALID", "许可证归属已调整，旧安装邀请失效")
		}
		if e = checkUsable(app, l, s.now()); e != nil {
			return e
		}
		if !contains(l.Terms.Forms, in.Form) || !contains(app.Forms, in.Form) {
			return forbidden("FORM_NOT_ALLOWED", "套餐未允许此部署形态")
		}
		if !contains(l.Terms.BindingTypes, in.Binding.Type) || !contains(app.AllowedBindings, in.Binding.Type) {
			return forbidden("BINDING_NOT_ALLOWED", "未允许此绑定类型")
		}
		b, e := s.runtimeBinding(app, in.Binding, l.Terms.MaxDomainAliases, req, publicHash)
		if e != nil {
			return e
		}
		if pending != nil && (pending.BindingHash != b.Hash || pending.BindingType != b.Kind) {
			return forbidden("BINDING_MISMATCH", "申请批准的绑定与当前安装不符")
		}
		rules, e := tx.Rules().Find(scoped, Filter{Limit: 10000})
		if e != nil {
			return e
		}
		if e = s.checkApplicationRules(app, rules, b, l.ID); e != nil {
			return e
		}
		existing, e := tx.Activations().Find(scoped, Filter{LicenseID: l.ID, PublicKeyHash: publicHash, ExactSessionID: &in.SessionID, Limit: 1})
		if e != nil {
			return e
		}
		var a Activation
		found := false
		for _, v := range existing {
			if v.SessionID == in.SessionID {
				a = v
				found = true
				break
			}
		}
		if found && a.DeactivatedAt == nil && (a.BindingType != b.Kind || a.BindingHash != b.Hash && b.Kind != "domain" && b.Kind != "ip") {
			return forbidden("BINDING_MISMATCH", "已有安装的绑定发生变化，请先执行换绑流程")
		}
		if !found {
			a = Activation{Record: s.record(in.AppID, "activation"), LicenseID: l.ID, InstallationID: publicHash, PublicKey: in.PublicKey, PublicKeyHash: publicHash, SessionID: in.SessionID, Label: in.Label}
		}
		now := s.now()
		active, e := tx.Activations().Find(scoped, Filter{LicenseID: l.ID, ActiveAt: &now, Limit: 10000})
		if e != nil {
			return e
		}
		others := 0
		for _, v := range active {
			if v.ID != a.ID {
				others++
			}
		}
		if l.Terms.Model == "floating" {
			if others >= l.Terms.MaxSeats {
				return forbidden("SEAT_EXCEEDED", "可用浮动席位已用完")
			}
			if a.Borrowed && a.LeaseExpiresAt != nil && now.Before(*a.LeaseExpiresAt) && in.BorrowSeconds == 0 {
				return conflict("SEAT_BORROWED", "此会话仍持有离线借用租约")
			}
			if e = s.extendLease(app, l, &a, in.BorrowSeconds); e != nil {
				return e
			}
		} else {
			if others >= l.Terms.MaxActivations {
				return forbidden("ACTIVATION_LIMIT_EXCEEDED", "活动安装数量超过套餐额度")
			}
			group, e := s.ensureAnchorGroup(scoped, tx, l, b)
			if e != nil {
				return e
			}
			if found && a.DeactivatedAt == nil && a.BindingGroupID != "" && a.BindingGroupID != group {
				return forbidden("BINDING_MISMATCH", "安装绑定组已改变")
			}
			a.BindingGroupID = group
		}
		a.BindingType = b.Kind
		a.BindingHash = b.Hash
		a.ClientBindingHash = b.ClientHash
		a.DeactivatedAt = nil
		a.DeactivationReason = ""
		a.LastVerifiedAt = now
		a.UpdatedAt = now
		if found {
			e = tx.Activations().Save(scoped, a)
		} else {
			e = tx.Activations().Insert(scoped, a)
		}
		if e != nil {
			return e
		}
		startLicense(&l, now)
		l.BindingType = b.Kind
		l.BindingHash = b.Hash
		l.UpdatedAt = now
		if e = tx.Licenses().Save(scoped, l); e != nil {
			return e
		}
		if card != nil {
			card.Status = "used"
			if card.UsedAt == nil {
				card.UsedAt = &now
			}
			card.UpdatedAt = now
			if e = tx.Cards().Save(scoped, *card); e != nil {
				return e
			}
		}
		if invitation != nil {
			invitation.UsedAt = &now
			invitation.PublicKeyHash = publicHash
			invitation.UpdatedAt = now
			if e = tx.Invitations().Save(scoped, *invitation); e != nil {
				return e
			}
		}
		if pending != nil {
			pending.ActivationID = a.ID
			pending.UpdatedAt = now
			if e = tx.PendingInstallations().Save(scoped, *pending); e != nil {
				return e
			}
		}
		out, e = s.runtimeTokens(scoped, tx, app, l, a)
		if e != nil {
			return e
		}
		return s.append(scoped, tx, "activated", a.ID, map[string]any{"license_id": l.ID, "binding_type": b.Kind, "form": in.Form, "restored": found})
	})
	if e != nil {
		return RuntimeResult{}, s.failure(scoped, repo, "activated", "", e)
	}
	return out, nil
}
func constantEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
func (s *Service) ensureAnchorGroup(ctx context.Context, tx Transaction, l License, b normalizedBinding) (string, error) {
	if b.Kind == "domain" || b.Kind == "ip" {
		rules, e := tx.Rules().Find(ctx, Filter{LicenseID: l.ID, Limit: 10000})
		if e != nil {
			return "", e
		}
		for _, rule := range rules {
			if rule.Kind == b.Kind && ruleMatches(rule, b.Value) {
				return rule.ID, nil
			}
		}
	}
	matches, e := tx.Anchors().Find(ctx, Filter{LicenseID: l.ID, Kind: b.Kind, BindingHash: b.Hash, Status: "active", Limit: 1})
	if e != nil {
		return "", e
	}
	group := ""
	values := b.Aliases
	if len(values) == 0 {
		values = []string{b.Value}
	}
	if len(matches) > 0 {
		group = matches[0].GroupID
		for _, value := range values {
			found, e := tx.Anchors().Count(ctx, Filter{LicenseID: l.ID, GroupID: group, Kind: b.Kind, BindingHash: s.hash("binding/"+b.Kind, l.ApplicationID, value), Status: "active"})
			if e != nil {
				return "", e
			}
			if found != 1 {
				return "", forbidden("BINDING_MISMATCH", "新域名不能自动加入已有绑定，请申请换绑")
			}
		}
		return group, nil
	}
	count, e := tx.CountBindingGroups(ctx, l.ID)
	if e != nil {
		return "", e
	}
	if count >= int64(l.Terms.MaxBindingGroups) {
		return "", forbidden("BINDING_LIMIT_EXCEEDED", "授权锚点组已用完，请申请换绑")
	}
	group = s.record(l.ApplicationID, "binding").ID
	for _, value := range values {
		hash := s.hash("binding/"+b.Kind, l.ApplicationID, value)
		sum := sha256.Sum256([]byte(b.Kind + "\x00" + value))
		anchor := BindingAnchor{Record: s.record(l.ApplicationID, "anchor"), LicenseID: l.ID, GroupID: group, Kind: b.Kind, BindingHash: hash, ClientHash: hex.EncodeToString(sum[:]), Display: maskAnchor(b.Kind, value), Status: "active"}
		old, e := tx.Anchors().Find(ctx, Filter{LicenseID: l.ID, Kind: b.Kind, BindingHash: hash, Limit: 1})
		if e != nil {
			return "", e
		}
		if len(old) > 0 {
			anchor.Record = old[0].Record
			anchor.UpdatedAt = s.now()
			e = tx.Anchors().Save(ctx, anchor)
		} else {
			e = tx.Anchors().Insert(ctx, anchor)
		}
		if e != nil {
			return "", e
		}
	}
	return group, nil
}
func (s *Service) extendLease(app Application, l License, a *Activation, borrow int) error {
	now := s.now()
	duration := app.VerifyPolicy.SeatLeaseSeconds
	a.Borrowed = false
	if borrow != 0 {
		if borrow < 1 || app.VerifyPolicy.MaxBorrowSeconds == 0 || borrow > app.VerifyPolicy.MaxBorrowSeconds {
			return invalid("离线借用时间超出应用策略")
		}
		duration = borrow
		a.Borrowed = true
	}
	until := now.Add(time.Duration(duration) * time.Second)
	if deadline := usageDeadline(app, l); deadline != nil && deadline.Before(until) {
		until = *deadline
	}
	if !now.Before(until) {
		return forbidden("LICENSE_EXPIRED", "授权已过期")
	}
	a.LeaseExpiresAt = &until
	return nil
}
func (s *Service) runtimeTokens(ctx context.Context, tx Transaction, app Application, l License, a Activation) (RuntimeResult, error) {
	now := s.now()
	expires := now.Add(time.Duration(app.VerifyPolicy.JWTTTLSeconds) * time.Second)
	grace := expires.Add(time.Duration(app.VerifyPolicy.NetworkGraceSeconds) * time.Second)
	for _, limit := range []*time.Time{usageDeadline(app, l), a.LeaseExpiresAt} {
		if limit != nil {
			if limit.Before(expires) {
				expires = *limit
			}
			if limit.Before(grace) {
				grace = *limit
			}
		}
	}
	if !now.Before(expires) || expires.Unix() <= now.Unix() {
		return RuntimeResult{}, forbidden("LICENSE_EXPIRED", "有效期不足，无法签发令牌")
	}
	refresh := now.Add(time.Duration(app.VerifyPolicy.VerifyIntervalSeconds) * time.Second)
	if l.Terms.Model == "floating" {
		refresh = now.Add(time.Duration(app.VerifyPolicy.SeatLeaseSeconds/3) * time.Second)
	}
	if !refresh.Before(expires) {
		refresh = expires.Add(-time.Second)
	}
	claims := JWTClaims{Issuer: TokenIssuer, Audience: app.ApplicationID, Subject: l.ID, JWTID: s.record(app.ApplicationID, "jwt").ID, IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: expires.Unix(), TokenUse: "license", ActivationID: a.ID, Binding: BindingClaim{a.BindingType, a.ClientBindingHash}, Model: l.Terms.Model, Perpetual: l.Terms.Perpetual, Features: l.Terms.Features, RefreshAfter: refresh.Unix(), GraceUntil: grace.Unix()}
	if l.UpdatesUntil != nil {
		v := l.UpdatesUntil.Unix()
		claims.UpdatesUntil = &v
	}
	token, e := s.signClaims(ctx, tx, app, claims)
	if e != nil {
		return RuntimeResult{}, e
	}
	out := RuntimeResult{AppID: app.ApplicationID, LicenseID: l.ID, ActivationID: a.ID, JWT: token, ServerTime: now, ExpiresAt: expires, RefreshAfter: refresh, GraceUntil: grace, LeaseExpiresAt: a.LeaseExpiresAt, UsageExpiresAt: l.ExpiresAt, UpdatesUntil: l.UpdatesUntil}
	if grace.After(expires) || a.Borrowed {
		claims.JWTID = s.record(app.ApplicationID, "offline").ID
		claims.TokenUse = "offline_grace"
		claims.ExpiresAt = grace.Unix()
		out.OfflineJWT, e = s.signClaims(ctx, tx, app, claims)
		if e != nil {
			return RuntimeResult{}, e
		}
	}
	if l.Terms.Model == "metered" {
		remaining := l.Terms.UsageQuota - l.UsedUnits
		if l.Terms.UsagePeriod == "monthly" && l.UsagePeriod != now.Format("2006-01") {
			remaining = l.Terms.UsageQuota
		}
		out.RemainingUnits = &remaining
	}
	return out, nil
}
func (s *Service) Verify(ctx context.Context, in VerifyInput, req SignedRequest) (RuntimeResult, error) {
	// The activation lookup locates a public verification key; it confers no
	// authority. Every subsequent operation requires its installation signature.
	lookup, _, repo, e := s.runtimeContext(ctx, in.AppID, Actor{Kind: "installation_lookup", ID: in.ActivationID})
	if e != nil {
		return RuntimeResult{}, e
	}
	var a Activation
	e = repo.View(lookup, func(v View) error { var e error; a, e = v.Activations().Get(lookup, in.ActivationID); return e })
	if e != nil {
		return RuntimeResult{}, forbidden("ACTIVATION_INVALID", "安装激活不存在")
	}
	scoped, _, repo, e := s.authenticateInstallation(ctx, in.AppID, a.PublicKey, req)
	if e != nil {
		return RuntimeResult{}, e
	}
	var out RuntimeResult
	e = repo.Update(scoped, func(tx Transaction) error {
		app, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		a, e = tx.Activations().Get(scoped, in.ActivationID)
		if e != nil {
			return e
		}

		l, e := tx.Licenses().Get(scoped, a.LicenseID)
		if e != nil {
			return e
		}
		action := in.Action
		if action == "" {
			action = "verify"
		}
		if action != "verify" && action != "release" && action != "consume" && action != "renew" {
			return invalid("校验动作无效")
		}
		reacquiring := a.DeactivatedAt != nil
		if reacquiring && a.DeactivationReason != "client_release" {
			return forbidden("ACTIVATION_DEACTIVATED", "安装已被管理员停用或迁移")
		}
		if reacquiring && action != "release" {
			a.DeactivatedAt = nil
			a.DeactivationReason = ""
			a.LeaseExpiresAt = nil
			a.Borrowed = false
		}
		if in.JWT != "" {
			_, e = s.verifyApplicationToken(scoped, tx.Keys(), app, in.JWT, TokenExpectation{ApplicationID: in.AppID, Subject: l.ID, ActivationID: a.ID, TokenUse: "license", Now: s.now()}, true)
			if e != nil {
				return e
			}
		}
		now := s.now()
		if action == "release" {
			if a.Borrowed && a.LeaseExpiresAt != nil && now.Before(*a.LeaseExpiresAt) {
				return conflict("BORROWED_SEAT_HELD", "离线借用席位将在借用到期后释放，不能提前复用")
			}
			a.DeactivatedAt = &now
			a.DeactivationReason = "client_release"
			a.LeaseExpiresAt = nil
			a.Borrowed = false
			a.UpdatedAt = now
			if e = tx.Activations().Save(scoped, a); e != nil {
				return e
			}
			out = RuntimeResult{AppID: in.AppID, LicenseID: l.ID, ActivationID: a.ID, ServerTime: now, Released: true}
			return s.append(scoped, tx, "seat_released", a.ID, map[string]any{"license_id": l.ID})
		}
		if action == "renew" {
			if e = s.redeemRenewal(scoped, tx, &l, in.RenewalCard); e != nil {
				return e
			}
		}
		if e = checkUsable(app, l, now); e != nil {
			return e
		}
		if !contains(l.Terms.Forms, in.Form) || !contains(app.Forms, in.Form) {
			return forbidden("FORM_NOT_ALLOWED", "此部署形态未获授权")
		}
		if !contains(app.AllowedBindings, in.Binding.Type) || in.Binding.Type != a.BindingType {
			return forbidden("BINDING_MISMATCH", "绑定类型不匹配")
		}
		b, e := s.runtimeBinding(app, in.Binding, l.Terms.MaxDomainAliases, req, a.PublicKeyHash)
		if e != nil {
			return e
		}
		rules, e := tx.Rules().Find(scoped, Filter{Limit: 10000})
		if e != nil {
			return e
		}
		if e = s.checkApplicationRules(app, rules, b, l.ID); e != nil {
			return e
		}
		ruleBound := false
		for _, rule := range rules {
			if rule.ID == a.BindingGroupID && rule.LicenseID == l.ID {
				if !ruleMatches(rule, b.Value) {
					return forbidden("BINDING_MISMATCH", "当前来源不匹配许可证的实时域名或 IP 规则")
				}
				ruleBound = true
			}
		}
		if strings.HasPrefix(a.BindingGroupID, "rule_") && !ruleBound {
			return forbidden("BINDING_MISMATCH", "许可证绑定规则已删除或停用")
		}
		if ruleBound {
			a.BindingHash = b.Hash
			a.ClientBindingHash = b.ClientHash
		} else if a.BindingType == "domain" {
			anchors, e := tx.Anchors().Find(scoped, Filter{LicenseID: l.ID, GroupID: a.BindingGroupID, Status: "active", BindingHash: b.Hash, Limit: 1})
			if e != nil {
				return e
			}
			if len(anchors) != 1 {
				return forbidden("BINDING_MISMATCH", "当前域名不属于此安装的已登记别名")
			}
			a.BindingHash = b.Hash
			a.ClientBindingHash = b.ClientHash
		} else if a.BindingHash != b.Hash {
			return forbidden("BINDING_MISMATCH", "当前绑定与激活时不一致")
		}
		if reacquiring && l.Terms.Model != "floating" {
			count, e := tx.Activations().Count(scoped, Filter{LicenseID: l.ID, ActiveAt: &now})
			if e != nil {
				return e
			}
			if count >= int64(l.Terms.MaxActivations) {
				return forbidden("ACTIVATION_LIMIT_EXCEEDED", "活动安装额度已被其他安装使用")
			}
		}
		if l.Terms.Model == "floating" {
			if a.Borrowed && a.LeaseExpiresAt != nil && now.Before(*a.LeaseExpiresAt) { /* retain the entire offline reservation */
			} else {
				count, e := tx.Activations().Count(scoped, Filter{LicenseID: l.ID, ActiveAt: &now, LeaseOnly: true})
				if e != nil {
					return e
				}
				own := a.LeaseExpiresAt != nil && now.Before(*a.LeaseExpiresAt)
				if !own && count >= int64(l.Terms.MaxSeats) {
					return forbidden("SEAT_EXCEEDED", "租约已过期且席位已被其他安装使用")
				}
				if e = s.extendLease(app, l, &a, 0); e != nil {
					return e
				}
			}
		}
		if action == "consume" {
			if e = s.consume(scoped, tx, &l, a, in.Units, in.IdempotencyKey); e != nil {
				return e
			}
		} else if in.Units != 0 || in.IdempotencyKey != "" {
			return invalid("计量参数只能用于 consume 动作")
		}
		a.LastVerifiedAt = now
		a.UpdatedAt = now
		if e = tx.Activations().Save(scoped, a); e != nil {
			return e
		}
		out, e = s.runtimeTokens(scoped, tx, app, l, a)
		if e != nil {
			return e
		}
		return s.append(scoped, tx, "verified", a.ID, map[string]any{"license_id": l.ID, "action": action})
	})
	if e != nil {
		return RuntimeResult{}, s.failure(scoped, repo, "verified", in.ActivationID, e)
	}
	return out, nil
}
func (s *Service) consume(ctx context.Context, tx Transaction, l *License, a Activation, units int64, key string) error {
	if l.Terms.Model != "metered" || units < 1 || units > 1_000_000_000_000 || key == "" || len(key) > 128 {
		return invalid("按量扣费需要正数额度与业务幂等键")
	}
	idempotency := s.hash("usage-key", l.ApplicationID, l.ID+"\x00"+key)
	requestHash := s.hash("usage-request", l.ApplicationID, key+"\x00"+strconv.FormatInt(units, 10))
	receipts, e := tx.Usage().Find(ctx, Filter{LicenseID: l.ID, IdempotencyKey: idempotency, Limit: 1})
	if e != nil {
		return e
	}
	if len(receipts) > 0 {
		if receipts[0].RequestHash != requestHash {
			return conflict("IDEMPOTENCY_CONFLICT", "幂等键已用于不同扣费内容")
		}
		return nil
	}
	period := "lifetime"
	if l.Terms.UsagePeriod == "monthly" {
		period = s.now().Format("2006-01")
	}
	if l.UsagePeriod != period {
		l.UsedUnits = 0
		l.UsagePeriod = period
	}
	if units > l.Terms.UsageQuota-l.UsedUnits {
		return forbidden("USAGE_EXCEEDED", "可用计量额度不足")
	}
	l.UsedUnits += units
	l.UpdatedAt = s.now()
	if e = tx.Licenses().Save(ctx, *l); e != nil {
		return e
	}
	receipt := UsageReceipt{Record: s.record(l.ApplicationID, "usage"), LicenseID: l.ID, ActivationID: a.ID, IdempotencyKey: idempotency, RequestHash: requestHash, Units: units, Period: period, Remaining: l.Terms.UsageQuota - l.UsedUnits}
	if e = tx.Usage().Insert(ctx, receipt); e != nil {
		return e
	}
	return s.append(ctx, tx, "usage_consumed", l.ID, map[string]any{"units": units, "period": period, "remaining": receipt.Remaining})
}

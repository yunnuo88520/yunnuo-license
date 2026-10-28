package licensing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
)

type Error struct {
	Code, Message string
	Status        int
}

func (e Error) Error() string              { return e.Code + ": " + e.Message }
func invalid(message string) error         { return Error{"INVALID_REQUEST", message, 400} }
func forbidden(code, message string) error { return Error{code, message, 403} }
func notFound() error                      { return Error{"NOT_FOUND", "当前应用中不存在该记录", 404} }
func conflict(code, message string) error  { return Error{code, message, 409} }
func errorCode(e error) string {
	var v Error
	if errors.As(e, &v) {
		return v.Code
	}
	var jwt *TokenError
	if errors.As(e, &jwt) {
		return jwt.Code
	}
	if errors.Is(e, ErrNotFound) {
		return "NOT_FOUND"
	}
	return "INTERNAL_ERROR"
}

type Principal struct {
	Actor   Actor
	AgentID string
}
type principalKey struct{}

// BindPrincipal is used by authenticated HTTP/session adapters, never decoded
// from a request body. Runtime activation has its own proof boundary.
func BindPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}
func PrincipalFrom(ctx context.Context) (Principal, error) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	if !ok || p.Actor.Kind == "" || p.Actor.ID == "" {
		return p, Error{"AUTHENTICATION_REQUIRED", "请先登录", 401}
	}
	return p, nil
}

type Service struct {
	releaseDir      string
	storageLimits   StorageLimits
	registry        Registry
	pepper, dataKey []byte
	now             func() time.Time
}

func New(registry Registry, pepper, dataKey []byte) *Service {
	return &Service{registry: registry, pepper: pepper, dataKey: dataKey, storageLimits: StorageLimits{TotalBytes: 16 << 30, DeveloperBytes: 8 << 30, ApplicationBytes: 2 << 30, Versions: 100}, now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) WithClock(clock func() time.Time) *Service {
	if clock != nil {
		s.now = clock
	}
	return s
}
func (s *Service) record(app, prefix string) Record {
	now := s.now()
	return Record{ApplicationID: app, ID: yncrypto.NewID(prefix), CreatedAt: now, UpdatedAt: now}
}
func (s *Service) hash(kind, app, value string) string {
	mac := hmac.New(sha256.New, s.pepper)
	mac.Write([]byte(kind + "\x00" + app + "\x00" + value))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Service) cryptoKey(app, purpose string) []byte {
	mac := hmac.New(sha256.New, s.dataKey)
	mac.Write([]byte("application-private-v2\x00" + app + "\x00" + purpose))
	return mac.Sum(nil)
}
func (s *Service) auditSigner(event AuditEvent) (string, error) {
	event.EventHash = ""
	raw, e := json.Marshal(event)
	if e != nil {
		return "", e
	}
	// Normalize nested structs and maps into the same canonical JSON tree.
	// UseNumber avoids changing large integer payloads during normalization.
	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if e = decoder.Decode(&canonical); e != nil {
		return "", e
	}
	raw, e = json.Marshal(canonical)
	if e != nil {
		return "", e
	}
	mac := hmac.New(sha256.New, s.dataKey)
	mac.Write([]byte("application-audit-v2\x00"))
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil)), nil
}
func (s *Service) event(ctx context.Context, kind, entity, result, reason string, payload map[string]any) AuditEvent {
	scope, _ := ScopeFrom(ctx)
	return AuditEvent{Record: s.record(scope.ApplicationID(), "event"), Actor: scope.Actor(), EventType: kind, EntityID: entity, Result: result, ReasonCode: reason, Payload: payload}
}
func (s *Service) append(ctx context.Context, tx Transaction, kind, entity string, payload map[string]any) error {
	return tx.AppendAudit(ctx, s.event(ctx, kind, entity, "success", "", payload), s.auditSigner)
}
func (s *Service) failure(ctx context.Context, repo Repository, kind, entity string, cause error) error {
	if cause == nil {
		return nil
	}
	e := repo.Update(ctx, func(tx Transaction) error {
		return tx.AppendAudit(ctx, s.event(ctx, kind, entity, "failed", errorCode(cause), nil), s.auditSigner)
	})
	if e != nil {
		return e
	}
	return cause
}
func privileged(p Principal, write bool) bool {
	if p.Actor.Kind == "system" {
		return true
	}
	if p.Actor.Kind != "admin" {
		return false
	}
	switch p.Actor.Role {
	case "super_admin", "admin":
		return true
	case "operator":
		return !write
	case "auditor":
		return !write
	}
	return false
}
func (s *Service) applicationContext(ctx context.Context, id string, write bool) (context.Context, Application, Repository, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return ctx, Application{}, nil, e
	}
	app, e := s.registry.GetApplication(ctx, id)
	if e != nil {
		return ctx, app, nil, notFound()
	}
	allowed := privileged(p, write) || p.Actor.Kind == "developer" && app.DeveloperID == p.Actor.ID || p.Actor.Kind == "application_server" && p.Actor.ID == app.ApplicationID
	if !allowed {
		return ctx, app, nil, notFound()
	}
	scoped, e := BindScope(ctx, app.ApplicationID, p.Actor, write)
	if e != nil {
		return ctx, app, nil, e
	}
	repo, e := s.registry.Scoped(scoped)
	return scoped, app, repo, e
}
func (s *Service) runtimeContext(ctx context.Context, id string, actor Actor) (context.Context, Application, Repository, error) {
	app, e := s.registry.GetApplication(ctx, id)
	if e != nil {
		return ctx, app, nil, Error{"APPLICATION_NOT_FOUND", "应用不存在或不可用", 404}
	}
	scoped, e := BindScope(ctx, app.ApplicationID, actor, true)
	if e != nil {
		return ctx, app, nil, e
	}
	repo, e := s.registry.Scoped(scoped)
	return scoped, app, repo, e
}
func (s *Service) ListApplications(ctx context.Context) ([]Application, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return nil, e
	}
	owner := ""
	if p.Actor.Kind == "developer" {
		owner = p.Actor.ID
	} else if !privileged(p, false) {
		return nil, forbidden("ROLE_FORBIDDEN", "无应用管理权限")
	}
	apps, e := s.registry.ListApplications(ctx, owner)
	for i := range apps {
		apps[i].SecretHash = ""
	}
	return apps, e
}

type CreateApplicationInput struct {
	BlockPolicy        *BlockPolicy `json:"block_policy,omitempty"`
	Group              string       `json:"group"`
	AllowOwnerTransfer *bool        `json:"allow_owner_transfer,omitempty"`
	UpdatesEnabled     *bool        `json:"updates_enabled,omitempty"`
	PublicQueryEnabled *bool        `json:"public_query_enabled,omitempty"`
	Name               string       `json:"name"`
	DeveloperID        string       `json:"developer_id"`
	AllowedModels      []string     `json:"allowed_models"`
	AllowedBindings    []string     `json:"allowed_bindings"`
	Forms              []string     `json:"forms"`
	VerifyPolicy       VerifyPolicy `json:"verify_policy"`
}
type CreatedApplication struct {
	Application  Application `json:"application"`
	AppID        string      `json:"app_id"`
	AppSecret    string      `json:"app_secret"`
	PublicKeyPEM string      `json:"public_key_pem"`
}

func (s *Service) CreateApplication(ctx context.Context, input CreateApplicationInput) (CreatedApplication, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return CreatedApplication{}, e
	}
	if p.Actor.Kind != "developer" && !privileged(p, true) {
		return CreatedApplication{}, forbidden("ROLE_FORBIDDEN", "无创建应用权限")
	}
	if p.Actor.Kind == "developer" {
		input.DeveloperID = p.Actor.ID
	}
	app := Application{BlockPolicy: BlockPolicy{Enabled: true, Announcement: "站点未授权，请联系软件提供方核实。"}, Group: strings.TrimSpace(input.Group), Name: strings.TrimSpace(input.Name), DeveloperID: input.DeveloperID, Status: "active", AllowedModels: input.AllowedModels, AllowedBindings: input.AllowedBindings, Forms: input.Forms, VerifyPolicy: input.VerifyPolicy, SecretVersion: 1, PublicQueryEnabled: true}
	if input.BlockPolicy != nil {
		app.BlockPolicy = *input.BlockPolicy
	}
	if input.AllowOwnerTransfer != nil {
		app.AllowOwnerTransfer = *input.AllowOwnerTransfer
	}
	if input.UpdatesEnabled != nil {
		app.UpdatesEnabled = *input.UpdatesEnabled
	}
	if input.PublicQueryEnabled != nil {
		app.PublicQueryEnabled = *input.PublicQueryEnabled
	}
	if e = normalizeApplication(&app); e != nil {
		return CreatedApplication{}, e
	}
	appID, e := yncrypto.ApplicationID()
	if e != nil {
		return CreatedApplication{}, e
	}
	secret, e := yncrypto.ApplicationSecret()
	if e != nil {
		return CreatedApplication{}, e
	}
	public, private, e := yncrypto.GenerateRSAKeyPair()
	if e != nil {
		return CreatedApplication{}, e
	}
	encrypted, e := yncrypto.EncryptString(s.cryptoKey(appID, "signing"), private)
	if e != nil {
		return CreatedApplication{}, e
	}
	app.Record = s.record(appID, "application")
	app.ID = appID
	app.CurrentKID = appID + ".v1"
	app.SecretHash = s.hash("app-secret", appID, secret)
	key := SigningKey{Record: s.record(appID, "key"), KID: app.CurrentKID, Version: 1, Algorithm: "RS256", PublicPEM: public, PrivateCipher: encrypted, Status: "active"}
	key.ID = key.KID
	event := AuditEvent{Record: s.record(appID, "event"), Actor: p.Actor, EventType: "created", EntityID: appID, Result: "success", Payload: map[string]any{"entity": "application", "name": app.Name}}
	if e = s.registry.CreateApplication(ctx, app, key, event, s.auditSigner); e != nil {
		return CreatedApplication{}, e
	}
	app.SecretHash = ""
	return CreatedApplication{app, appID, secret, public}, nil
}
func (s *Service) GetApplication(ctx context.Context, id string) (Application, error) {
	_, app, _, e := s.applicationContext(ctx, id, false)
	app.SecretHash = ""
	return app, e
}
func (s *Service) SaveApplication(ctx context.Context, id string, input CreateApplicationInput, status string) (Application, error) {
	scoped, app, repo, e := s.applicationContext(ctx, id, true)
	if e != nil {
		return app, e
	}
	app.Group = strings.TrimSpace(input.Group)
	app.Name = input.Name
	app.AllowedModels = input.AllowedModels
	app.AllowedBindings = input.AllowedBindings
	app.Forms = input.Forms
	app.VerifyPolicy = input.VerifyPolicy
	if input.BlockPolicy != nil {
		app.BlockPolicy = *input.BlockPolicy
	}
	if input.AllowOwnerTransfer != nil {
		app.AllowOwnerTransfer = *input.AllowOwnerTransfer
	}
	if input.UpdatesEnabled != nil {
		app.UpdatesEnabled = *input.UpdatesEnabled
	}
	if input.PublicQueryEnabled != nil {
		app.PublicQueryEnabled = *input.PublicQueryEnabled
	}
	if status != "" {
		app.Status = status
	}
	if e = normalizeApplication(&app); e != nil {
		return app, e
	}
	app.UpdatedAt = s.now()
	e = repo.Update(scoped, func(tx Transaction) error {
		if e := tx.LockApplication(scoped); e != nil {
			return e
		}
		current, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		if current.SecretVersion != app.SecretVersion {
			return conflict("APPLICATION_CHANGED", "应用凭证已改变，请刷新后重试")
		}
		current.BlockPolicy = app.BlockPolicy
		current.Group = app.Group
		current.Name = app.Name
		current.Status = app.Status
		current.AllowedModels = app.AllowedModels
		current.AllowedBindings = app.AllowedBindings
		current.Forms = app.Forms
		current.AllowOwnerTransfer = app.AllowOwnerTransfer
		current.UpdatesEnabled = app.UpdatesEnabled
		current.PublicQueryEnabled = app.PublicQueryEnabled
		current.VerifyPolicy = app.VerifyPolicy
		current.UpdatedAt = app.UpdatedAt
		app = current
		if e := tx.SaveApplication(scoped, app); e != nil {
			return e
		}
		return s.append(scoped, tx, "application_updated", id, map[string]any{"status": app.Status})
	})
	app.SecretHash = ""
	return app, e
}
func (s *Service) ResetAppSecret(ctx context.Context, id, reason string) (map[string]any, error) {
	scoped, app, repo, e := s.applicationContext(ctx, id, true)
	if e != nil {
		return nil, e
	}
	if strings.TrimSpace(reason) == "" {
		return nil, invalid("请填写重置原因")
	}
	secret, e := yncrypto.ApplicationSecret()
	if e != nil {
		return nil, e
	}
	app.SecretHash = s.hash("app-secret", id, secret)
	app.SecretVersion++
	app.UpdatedAt = s.now()
	e = repo.Update(scoped, func(tx Transaction) error {
		if e := tx.LockApplication(scoped); e != nil {
			return e
		}
		current, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		if current.SecretVersion != app.SecretVersion-1 {
			return conflict("APPLICATION_CHANGED", "应用凭证已改变，请刷新后重试")
		}
		current.SecretHash = app.SecretHash
		current.SecretVersion = app.SecretVersion
		current.UpdatedAt = app.UpdatedAt
		app = current
		if e := tx.SaveApplication(scoped, app); e != nil {
			return e
		}
		return s.append(scoped, tx, "app_secret_rotated", id, map[string]any{"reason": reason, "version": app.SecretVersion})
	})
	return map[string]any{"app_id": id, "app_secret": secret, "version": app.SecretVersion}, e
}
func (s *Service) AuthenticateAppSecret(ctx context.Context, appID, secret string) (Application, error) {
	app, e := s.registry.GetApplication(ctx, appID)
	if e != nil || app.Status != "active" || len(secret) != 32 || subtle.ConstantTimeCompare([]byte(app.SecretHash), []byte(s.hash("app-secret", appID, secret))) != 1 {
		return Application{}, Error{"APPLICATION_CREDENTIALS_INVALID", "应用凭证无效", 401}
	}
	return app, nil
}

func (s *Service) WithReleaseStorage(dir string, limits StorageLimits) *Service {
	s.releaseDir = dir
	if limits.TotalBytes > 0 && limits.DeveloperBytes > 0 && limits.ApplicationBytes > 0 && limits.Versions > 0 {
		s.storageLimits = limits
	}
	return s
}

func (s *Service) CheckApplicationAccess(ctx context.Context, id string, write bool) error {
	_, _, _, e := s.applicationContext(ctx, id, write)
	return e
}

func (s *Service) publicApplicationContext(ctx context.Context, id string) (context.Context, Application, Repository, error) {
	app, e := s.registry.GetApplication(ctx, id)
	if e != nil || app.Status != "active" || !app.PublicQueryEnabled {
		return ctx, app, nil, notFound()
	}
	scoped, e := BindScope(ctx, id, Actor{Kind: "public_query", ID: "anonymous"}, false)
	if e != nil {
		return ctx, app, nil, e
	}
	repo, e := s.registry.Scoped(scoped)
	return scoped, app, repo, e
}

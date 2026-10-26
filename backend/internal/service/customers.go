package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

var (
	ErrCustomerToken    = unauthorized("INVALID_USER_TOKEN", "invalid user session")
	ErrCustomerDisabled = forbidden("USER_DISABLED", "user account is disabled")
	ErrOwnerRequired    = forbidden("OWNER_REQUIRED", "activated licenses must be assigned by an administrator")
	ErrRebindDenied     = forbidden("REBIND_DISABLED", "binding changes are disabled for this application")
	ErrRebindLimit      = forbidden("REBIND_LIMIT_REACHED", "binding change allowance exhausted")
)

type CustomerInput struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}
type CustomerLoginResponse struct {
	AccessToken string          `json:"access_token"`
	User        domain.Customer `json:"user"`
	ExpiresAt   time.Time       `json:"expires_at"`
}

func (s *Service) transactionAudit(ctx context.Context, tx *store.Tx, kind, id, product, license, action string, extra any) error {
	raw, err := json.Marshal(extra)
	if err != nil {
		return err
	}
	meta := ActorFrom(ctx)
	requestID, ip := "", ""
	if meta != nil {
		requestID = meta.RequestID
		ip = meta.ClientIP
	}
	return tx.InsertAudit(ctx, domain.AuditLog{RequestID: requestID, ClientIP: ip, ID: yncrypto.NewID("audit"), ActorType: kind, ActorID: id, ProductID: product, LicenseID: license, Action: action, Result: "success", ExtraJSON: string(raw), CreatedAt: s.now()})
}

func (s *Service) CreateCustomer(ctx context.Context, input CustomerInput, actorID string) (domain.Customer, error) {
	input.Username = normalizeUsername(input.Username)
	if input.Username == "" || len(input.Username) > 80 || len(input.Password) < 8 || len(input.Password) > 256 || len(input.DisplayName) > 160 {
		return domain.Customer{}, ErrInvalidRequest
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return domain.Customer{}, err
	}
	if err := s.ensureEmailAvailable(ctx, "customer_users", email); err != nil {
		return domain.Customer{}, err
	}
	hash, err := hashAgentPassword(input.Password)
	if err != nil {
		return domain.Customer{}, err
	}
	c := domain.Customer{ID: yncrypto.NewID("usr"), Username: input.Username, Email: email, DisplayName: strings.TrimSpace(input.DisplayName), PasswordHash: hash, Status: "active", SessionVersion: 1, CreatedAt: s.now()}
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		available, err := tx.IdentityLoginAvailable(ctx, c.Username, c.Email)
		if err != nil {
			return err
		}
		if !available {
			return AppError{Code: "USERNAME_TAKEN", Message: "登录账号或邮箱已存在，请选择已有用户", HTTPStatus: 409}
		}
		if err := tx.CreateCustomer(ctx, c); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actorID, "", "", "user.create", map[string]string{"user_id": c.ID})
	})
	return c, err
}
func (s *Service) Customers(ctx context.Context) ([]domain.Customer, error) {
	return s.currentStore().Customers(ctx)
}
func (s *Service) CustomerLogin(ctx context.Context, input AdminLoginInput) (CustomerLoginResponse, error) {
	if err := s.ensureLoginCaptcha(ctx, input.CaptchaID, input.CaptchaCode); err != nil {
		return CustomerLoginResponse{}, err
	}
	c, err := s.currentStore().CustomerByLogin(ctx, normalizeUsername(input.Username))
	if err != nil {
		return CustomerLoginResponse{}, ErrInvalidCredentials
	}
	if c.Status != "active" {
		return CustomerLoginResponse{}, ErrCustomerDisabled
	}
	ok, err := verifyAgentPassword(input.Password, c.PasswordHash)
	if err != nil || !ok {
		return CustomerLoginResponse{}, ErrInvalidCredentials
	}
	expiry := s.now().Add(12 * time.Hour)
	token, err := s.signAdminToken(adminTokenPayload{Type: "customer_session", UserID: c.ID, SessionVersion: c.SessionVersion, IssuedAt: s.now().Unix(), ExpiresAt: expiry.Unix()})
	if err == nil {
		c, err = s.UserAccountProfile(ctx, c.ID)
	}
	return CustomerLoginResponse{AccessToken: token, User: c, ExpiresAt: expiry}, err
}

func (s *Service) UserAccountProfile(ctx context.Context, id string) (domain.Customer, error) {
	c, err := s.currentStore().CustomerByID(ctx, id)
	if err != nil {
		return c, err
	}
	m, err := s.currentStore().AgentMemberForUser(ctx, id)
	if store.IsNotFound(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	a, err := s.currentStore().GetAgent(ctx, m.AgentID)
	if err != nil {
		return c, err
	}
	c.AgentID = a.ID
	c.AgentStatus = a.Status
	if m.Status != domain.AgentUserActive {
		c.AgentStatus = "disabled"
	}
	return c, nil
}
func (s *Service) AuthenticateCustomer(ctx context.Context, token string) (domain.Customer, error) {
	claim, err := s.verifyAdminToken(token)
	if err != nil || claim.Type != "customer_session" || !s.now().Before(time.Unix(claim.ExpiresAt, 0)) {
		return domain.Customer{}, ErrCustomerToken
	}
	c, err := s.currentStore().CustomerByID(ctx, claim.UserID)
	if err != nil || c.SessionVersion != claim.SessionVersion {
		return domain.Customer{}, ErrCustomerToken
	}
	if c.Status != "active" {
		return domain.Customer{}, ErrCustomerDisabled
	}
	return c, nil
}
func (s *Service) SetCustomerStatus(ctx context.Context, id, status, actor string) error {
	if status != "active" && status != "disabled" {
		return ErrInvalidRequest
	}
	if _, err := s.currentStore().CustomerByID(ctx, id); err != nil {
		return notFound("USER_NOT_FOUND", "user not found")
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.SetCustomerStatus(ctx, id, status); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "user.status", map[string]string{"user_id": id, "status": status})
	})
}
func (s *Service) CustomerPassword(ctx context.Context, id string, input ChangeAdminPasswordInput) error {
	if len(input.NewPassword) < 8 || len(input.NewPassword) > 256 {
		return ErrInvalidRequest
	}
	c, err := s.currentStore().CustomerByID(ctx, id)
	if err != nil {
		return ErrCustomerToken
	}
	ok, err := verifyAgentPassword(input.CurrentPassword, c.PasswordHash)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	hash, err := hashAgentPassword(input.NewPassword)
	if err != nil {
		return err
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.SetCustomerPassword(ctx, id, hash); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "customer", id, "", "", "user.password", nil)
	})
}

func (s *Service) CustomerActivate(ctx context.Context, userID, code string, input ActivateInput) (LicenseResponse, error) {
	card, err := s.currentStore().GetCardByHash(ctx, yncrypto.CardHash(s.cardPepper, input.CardCode))
	if err != nil {
		if store.IsNotFound(err) {
			return LicenseResponse{}, ErrCardInvalid
		}
		return LicenseResponse{}, err
	}
	p, err := s.currentStore().GetProduct(ctx, card.ProductID)
	if err != nil {
		return LicenseResponse{}, err
	}
	// An optional application ID (or legacy code) must agree with the stored card.
	code = strings.TrimSpace(code)
	if code != "" && code != p.ID && !strings.EqualFold(code, p.Code) {
		return LicenseResponse{}, ErrCardInvalid
	}
	input.AppKey = p.AppKey
	input.OwnerUserID = userID
	return s.Activate(ctx, input)
}
func (s *Service) CustomerLicense(ctx context.Context, userID, no string) (domain.CustomerLicense, error) {
	var view domain.CustomerLicense
	lic, err := s.currentStore().GetLicenseByNo(ctx, no)
	if err != nil || lic.OwnerUserID != userID || userID == "" {
		return view, ErrLicenseNotFound
	}
	p, err := s.currentStore().GetProduct(ctx, lic.ProductID)
	if err != nil {
		return view, err
	}
	p.PrivateKeyEncrypted = ""
	p.DeveloperID = ""
	bindings, err := s.currentStore().GetBindings(ctx, lic.ID)
	if err != nil {
		return view, err
	}
	bindingOnlineStates(bindings, p.OfflineGraceDays, s.now())
	view.Bindings = []domain.CustomerBinding{}
	for _, b := range bindings {
		value, err := yncrypto.DecryptString(s.dataKey, b.BindValueEncrypted)
		if err != nil {
			return view, err
		}
		view.Bindings = append(view.Bindings, domain.CustomerBinding{
			ID: b.ID, Mode: b.BindMode, Value: value, Status: b.Status,
			LastSeenAt: b.LastSeenAt, OnlineState: b.OnlineState, OfflineDays: b.OfflineDays,
		})
	}
	view.CanUse = p.Status == domain.ProductActive && s.ensureUsable(p, lic, s.now()) == nil
	view.CanUpdate = p.Status == domain.ProductActive && licenseCanUpdate(lic, s.now())
	if lic.Status == domain.LicenseActive && lic.ExpiredAt != nil && !s.now().Before(*lic.ExpiredAt) {
		lic.Status = domain.LicenseExpired
	}
	action, _, _, securityErr := securityAction(ctx, s.currentStore(), lic)
	if securityErr != nil {
		return view, securityErr
	}
	if action == "deny_use" || action == "block_page" {
		view.CanUse = false
	}
	if action != "observe" {
		view.CanUpdate = false
	}
	view.License = lic
	view.Product = p
	view.Policy, err = s.currentStore().BindingPolicy(ctx, p)
	if err != nil {
		return view, err
	}
	view.ChangesUsed, view.LastChangeAt, err = s.currentStore().BindingChangeStats(ctx, lic.ID)
	return view, err
}
func (s *Service) CustomerLicenses(ctx context.Context, userID string) ([]domain.CustomerLicense, error) {
	items, err := s.currentStore().CustomerLicenses(ctx, userID)
	if err != nil {
		return nil, err
	}
	views := []domain.CustomerLicense{}
	for _, lic := range items {
		view, err := s.CustomerLicense(ctx, userID, lic.LicenseNo)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}
func (s *Service) CustomerRenew(ctx context.Context, userID, no, bindingID, card string) (LicenseResponse, error) {
	view, err := s.CustomerLicense(ctx, userID, no)
	if err != nil {
		return LicenseResponse{}, err
	}
	for _, b := range view.Bindings {
		if b.ID == bindingID && b.Status == domain.BindingActive {
			return s.Renew(ctx, RenewInput{OwnerUserID: userID, AppKey: view.Product.AppKey, LicenseNo: no, BindMode: b.Mode, BindValue: b.Value, RenewCardCode: card})
		}
	}
	return LicenseResponse{}, ErrBindingMismatch
}

func (s *Service) BindingPolicy(ctx context.Context, productID string) (domain.BindingPolicy, error) {
	p, err := s.currentStore().GetProduct(ctx, productID)
	if err != nil {
		return domain.BindingPolicy{}, ErrProductNotFound
	}
	return s.currentStore().BindingPolicy(ctx, p)
}
func (s *Service) SetBindingPolicy(ctx context.Context, productID, actor string, input domain.BindingPolicy) (domain.BindingPolicy, error) {
	p, err := s.currentStore().GetProduct(ctx, productID)
	if err != nil {
		return input, ErrProductNotFound
	}
	if input.Limit < -1 || input.Limit > 10000 || input.CooldownHours < 0 || input.CooldownHours > 87600 || (input.Allowed && (input.Limit == 0 || p.BindMode == domain.BindNone)) {
		return input, ErrInvalidRequest
	}
	input.ProductID = p.ID
	input.BindMode = p.BindMode
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.SetBindingPolicy(ctx, input); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, p.ID, "", "binding.policy", input)
	})
	return input, err
}
func (s *Service) guardBindingChange(ctx context.Context, tx *store.Tx, p domain.Product, lic domain.License) error {
	policy, err := tx.BindingPolicy(ctx, p)
	if err != nil {
		return err
	}
	if !policy.Allowed || p.BindMode == domain.BindNone {
		return ErrRebindDenied
	}
	count, last, err := tx.BindingChangeStats(ctx, lic.ID)
	if err != nil {
		return err
	}
	if policy.Limit >= 0 && count >= policy.Limit {
		return ErrRebindLimit
	}
	if last != nil && s.now().Before(last.Add(time.Duration(policy.CooldownHours)*time.Hour)) {
		return ErrUnbindCooldown
	}
	return s.ensureUsable(p, lic, s.now())
}

type RebindInput struct {
	BindingID string `json:"binding_id"`
	NewValue  string `json:"new_value"`
	RequestID string `json:"request_id"`
	Reason    string `json:"reason"`
}

func (s *Service) CustomerRebind(ctx context.Context, userID, no string, input RebindInput) (domain.CustomerLicense, error) {
	view, err := s.CustomerLicense(ctx, userID, no)
	if err != nil {
		return view, err
	}
	p := view.Product
	if p.Status != domain.ProductActive {
		return view, ErrProductDisabled
	}
	if input.RequestID == "" || len(input.RequestID) > 64 || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 500 {
		return view, ErrInvalidRequest
	}
	_, value, hash, err := s.normalizeBinding(p, p.BindMode, input.NewValue)
	if err != nil {
		return view, err
	}
	encrypted, err := yncrypto.EncryptString(s.dataKey, value)
	if err != nil {
		return view, err
	}
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockLicense(ctx, view.License.ID); err != nil {
			return err
		}
		owner, err := tx.Owner(ctx, view.License.ID)
		if err != nil {
			return err
		}
		if owner != userID {
			return ErrLicenseNotFound
		}
		lic, err := tx.GetLicenseByID(ctx, view.License.ID)
		if err != nil {
			return err
		}
		if previous, e := tx.BindingChange(ctx, lic.ID, input.RequestID); e == nil {
			if previous != hash {
				return badRequest("IDEMPOTENCY_CONFLICT", "request ID belongs to another change")
			}
			return nil
		} else if !store.IsNotFound(e) {
			return e
		}
		binding, err := tx.GetBindingByID(ctx, input.BindingID)
		if err != nil || binding.LicenseID != lic.ID || binding.Status != domain.BindingActive {
			return ErrBindingMismatch
		}
		if binding.BindValueHash == hash {
			return nil
		}
		if err := s.guardBindingChange(ctx, tx, p, lic); err != nil {
			return err
		}
		if _, err = tx.ActiveBindingByHash(ctx, lic.ID, p.BindMode, hash); err == nil {
			return badRequest("BINDING_EXISTS", "binding already exists")
		} else if !store.IsNotFound(err) {
			return err
		}
		if err := tx.ReplaceBinding(ctx, binding.ID, hash, encrypted, s.now()); err != nil {
			return err
		}
		if err := tx.AddBindingChange(ctx, yncrypto.NewID("change"), lic.ID, binding.ID, input.RequestID, hash, "replace", s.now()); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "customer", userID, p.ID, lic.ID, "binding.replace", map[string]string{"binding_id": binding.ID, "reason": input.Reason})
	})
	if err != nil {
		return view, err
	}
	return s.CustomerLicense(ctx, userID, no)
}

type AssignOwnerInput struct {
	UserID        string `json:"user_id"`
	ExpectedOwner string `json:"expected_owner"`
	Reason        string `json:"reason"`
}

func (s *Service) AssignOwner(ctx context.Context, no, actor string, input AssignOwnerInput) error {
	if strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 500 {
		return ErrInvalidRequest
	}
	c, err := s.currentStore().CustomerByID(ctx, input.UserID)
	if err != nil || c.Status != "active" {
		return notFound("USER_NOT_FOUND", "active user not found")
	}
	lic, err := s.currentStore().GetLicenseByNo(ctx, no)
	if err != nil {
		return ErrLicenseNotFound
	}
	p, err := s.currentStore().GetProduct(ctx, lic.ProductID)
	if err != nil {
		return err
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockLicense(ctx, lic.ID); err != nil {
			return err
		}
		owner, err := tx.Owner(ctx, lic.ID)
		if err != nil {
			return err
		}
		if owner != input.ExpectedOwner {
			return AppError{Code: "OWNER_CHANGED", Message: "license owner changed; refresh before retrying", HTTPStatus: 409}
		}
		if owner == input.UserID {
			return nil
		}
		policy, err := tx.BindingPolicy(ctx, p)
		if err != nil {
			return err
		}
		if owner != "" && !policy.AllowOwnerTransfer {
			return forbidden("OWNER_TRANSFER_DISABLED", "owner transfers are disabled")
		}
		if err := tx.SetOwner(ctx, lic.ID, input.UserID, s.now()); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, p.ID, lic.ID, "license.owner", map[string]string{"from": owner, "to": input.UserID, "reason": input.Reason})
	})
}

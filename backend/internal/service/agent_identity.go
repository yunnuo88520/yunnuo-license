package service

import (
	"context"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func (s *Service) prepareAgentIdentity(ctx context.Context, in CreateAgentUserInput) (domain.Customer, bool, error) {
	if in.UserID != "" {
		if in.Username != "" || in.Password != "" || in.Email != "" {
			return domain.Customer{}, false, ErrInvalidRequest
		}
		c, err := s.currentStore().CustomerByID(ctx, in.UserID)
		if err != nil || c.Status != "active" {
			return c, false, ErrInvalidRequest
		}
		return c, false, nil
	}
	username := normalizeUsername(in.Username)
	if username == "" || len(username) > 80 || len(in.Password) < 8 || len(in.Password) > 256 || len(in.DisplayName) > 160 {
		return domain.Customer{}, false, ErrInvalidRequest
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return domain.Customer{}, false, err
	}
	hash, err := hashAgentPassword(in.Password)
	if err != nil {
		return domain.Customer{}, false, err
	}
	return domain.Customer{ID: yncrypto.NewID("usr"), Username: username, PasswordHash: hash, Email: email, DisplayName: strings.TrimSpace(in.DisplayName), Status: "active", SessionVersion: 1, CreatedAt: s.now()}, true, nil
}

func (s *Service) createAgentMembership(ctx context.Context, tx *store.Tx, c domain.Customer, newUser bool, agent, role, phone string) (domain.AgentUser, error) {
	if newUser {
		ok, err := tx.IdentityLoginAvailable(ctx, c.Username, c.Email)
		if err != nil {
			return domain.AgentUser{}, err
		}
		if !ok {
			return domain.AgentUser{}, AppError{Code: "USERNAME_TAKEN", Message: "登录账号或邮箱已存在，请选择已有用户", HTTPStatus: 409}
		}
		if err = tx.CreateCustomer(ctx, c); err != nil {
			return domain.AgentUser{}, err
		}
	} else {
		var err error
		if err = tx.LockAccount(ctx, "customer", c.ID); err != nil {
			return domain.AgentUser{}, err
		}
		c, err = tx.CustomerByID(ctx, c.ID)
		if err != nil || c.Status != "active" {
			return domain.AgentUser{}, ErrInvalidRequest
		}
	}
	if _, err := tx.AgentMemberForUser(ctx, c.ID); err == nil {
		return domain.AgentUser{}, AppError{Code: "AGENT_ALREADY_GRANTED", Message: "该用户已开通代理资格或属于代理团队", HTTPStatus: 409}
	} else if !store.IsNotFound(err) {
		return domain.AgentUser{}, err
	}
	now := s.now()
	m := domain.AgentUser{ID: c.ID, IdentityID: c.ID, AgentID: agent, Username: c.Username, PasswordHash: c.PasswordHash, Email: c.Email, DisplayName: c.DisplayName, Phone: strings.TrimSpace(phone), Role: role, Status: domain.AgentUserActive, CreatedAt: now, UpdatedAt: now}
	if err := tx.CreateAgentUser(ctx, m); err != nil {
		return m, err
	}
	if err := tx.BindAgentIdentity(ctx, m.ID, c.ID); err != nil {
		return m, err
	}
	return m, nil
}

func (s *Service) createUnifiedAgentMember(ctx context.Context, in CreateAgentUserInput) (domain.AgentUser, error) {
	if in.Role == "" {
		in.Role = domain.AgentRoleOwner
	}
	if !validAgentRole(in.Role) {
		return domain.AgentUser{}, ErrInvalidRequest
	}
	a, err := s.currentStore().GetAgent(ctx, in.AgentID)
	if err != nil {
		return domain.AgentUser{}, ErrAgentNotFound
	}
	if err = ensureAgentActive(a); err != nil {
		return domain.AgentUser{}, err
	}
	c, newUser, err := s.prepareAgentIdentity(ctx, in)
	if err != nil {
		return domain.AgentUser{}, err
	}
	var member domain.AgentUser
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		var e error
		member, e = s.createAgentMembership(ctx, tx, c, newUser, a.ID, in.Role, in.Phone)
		if e != nil {
			return e
		}
		actor := ""
		if meta := ActorFrom(ctx); meta != nil {
			actor = meta.ID
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "agent.member.create", map[string]string{"agent_id": a.ID, "user_id": c.ID})
	})
	if err == nil {
		s.notifyAgentAccountCreated(a.ID, member, a.Name, "")
	}
	return member, err
}

func (s *Service) unifiedAgentLogin(ctx context.Context, in AgentLoginInput) (AgentLoginResponse, error) {
	login, err := s.CustomerLogin(ctx, AdminLoginInput{Username: in.Username, Password: in.Password, CaptchaID: in.CaptchaID, CaptchaCode: in.CaptchaCode})
	if err != nil {
		return AgentLoginResponse{}, err
	}
	m, err := s.currentStore().AgentMemberForUser(ctx, login.User.ID)
	if err != nil {
		return AgentLoginResponse{}, ErrPermissionDenied
	}
	a, err := s.currentStore().GetAgent(ctx, m.AgentID)
	if err != nil {
		return AgentLoginResponse{}, ErrAgentNotFound
	}
	if in.AgentID != "" && in.AgentID != a.ID || in.AgentNo != "" && !strings.EqualFold(strings.TrimSpace(in.AgentNo), a.AgentNo) {
		return AgentLoginResponse{}, ErrInvalidCredentials
	}
	if in.LoginCode != "" {
		code, e := s.currentStore().GetAgentLoginCode(ctx, a.ID)
		if e != nil || normalizeAgentLoginCode(in.LoginCode) != code {
			return AgentLoginResponse{}, ErrInvalidCredentials
		}
	}
	if _, err = s.agentSessionForMember(ctx, m, login.ExpiresAt); err != nil {
		return AgentLoginResponse{}, err
	}
	now := s.now()
	if err = s.currentStore().UpdateAgentUserLastLogin(ctx, m.ID, now); err != nil {
		return AgentLoginResponse{}, err
	}
	m.LastLoginAt = &now
	m.IdentityID = login.User.ID
	_ = s.auditAgent(ctx, m.ID, a.ID, "", "agent.login", "success", "")
	return AgentLoginResponse{AccessToken: login.AccessToken, TokenType: "Bearer", ExpiresAt: login.ExpiresAt, Agent: a, User: m}, nil
}

func (s *Service) agentSessionForMember(ctx context.Context, m domain.AgentUser, expires time.Time) (AgentSession, error) {
	if m.Status != domain.AgentUserActive {
		return AgentSession{}, ErrAgentUserDisabled
	}
	a, err := s.currentStore().GetAgent(ctx, m.AgentID)
	if err != nil {
		return AgentSession{}, ErrAgentNotFound
	}
	if err = ensureAgentActive(a); err != nil {
		return AgentSession{}, err
	}
	return AgentSession{AgentID: a.ID, AgentNo: a.AgentNo, AgentName: a.Name, UserID: m.ID, Username: m.Username, DisplayName: m.DisplayName, Role: m.Role, ExpiresAt: expires}, nil
}

func (s *Service) EligibleAgentUsers(ctx context.Context) ([]domain.Customer, error) {
	return s.currentStore().EligibleAgentUsers(ctx)
}

type OpenAgentInput struct {
	UserID           string   `json:"user_id"`
	Username         string   `json:"username"`
	Password         string   `json:"password"`
	DisplayName      string   `json:"display_name"`
	Email            string   `json:"email"`
	Name             string   `json:"name"`
	Phone            string   `json:"phone"`
	TierID           string   `json:"tier_id"`
	PackageIDs       []string `json:"package_ids"`
	GroupIDs         []string `json:"group_ids"`
	MaxBatchQuantity int      `json:"max_batch_quantity"`
	CanExport        bool     `json:"can_export"`
	InitialBalance   int64    `json:"initial_balance"`
	Reason           string   `json:"reason"`
}
type OpenAgentResult struct {
	Agent    domain.Agent        `json:"agent"`
	User     domain.Customer     `json:"user"`
	Account  domain.AgentAccount `json:"account"`
	LoginURL string              `json:"login_url"`
	UserURL  string              `json:"user_url"`
}

func (s *Service) OpenAgentAccount(ctx context.Context, in OpenAgentInput, actor string) (OpenAgentResult, error) {
	var out OpenAgentResult
	if in.MaxBatchQuantity == 0 {
		in.MaxBatchQuantity = 100
	}
	if len(in.PackageIDs)+len(in.GroupIDs) > 500 || in.MaxBatchQuantity < 1 || in.MaxBatchQuantity > 1000 || in.InitialBalance < 0 || in.InitialBalance > maxMoney || in.InitialBalance > 0 && strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 {
		return out, ErrInvalidRequest
	}
	c, newUser, err := s.prepareAgentIdentity(ctx, CreateAgentUserInput{UserID: in.UserID, Username: in.Username, Password: in.Password, DisplayName: in.DisplayName, Email: in.Email})
	if err != nil {
		return out, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = nonEmpty(c.DisplayName, c.Username)
	}
	if len(name) > 160 {
		return out, ErrInvalidRequest
	}
	code, err := s.newAgentLoginCode(ctx)
	if err != nil {
		return out, err
	}
	now := s.now()
	a := domain.Agent{ID: yncrypto.NewID("agent"), AgentNo: strings.ToUpper(yncrypto.NewID("agt")), LoginCode: code, Name: name, ContactName: c.DisplayName, Phone: strings.TrimSpace(in.Phone), Email: c.Email, Level: 1, Status: domain.AgentActive, SettlementMode: domain.SettlementManual, DefaultDiscountRate: 1, CreatedBy: actor, CreatedAt: now, UpdatedAt: now, OwnerUserID: c.ID, Username: c.Username, MemberCount: 1}
	account := domain.AgentAccount{AgentID: a.ID, TierID: in.TierID, Balance: in.InitialBalance}
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.CreateAgentWithLoginCode(ctx, a); err != nil {
			return err
		}
		if _, err := s.createAgentMembership(ctx, tx, c, newUser, a.ID, domain.AgentRoleOwner, in.Phone); err != nil {
			return err
		}
		if err := tx.SaveAccount(ctx, account); err != nil {
			if store.IsNotFound(err) {
				return badRequest("AGENT_TIER_NOT_FOUND", "代理等级不存在，请重新选择")
			}
			return err
		}
		packages := map[string]domain.CardPackage{}
		for _, id := range in.PackageIDs {
			p, e := tx.Package(ctx, id)
			if e != nil || !p.Enabled {
				return ErrPackageDenied
			}
			packages[p.ID] = p
		}
		for _, id := range in.GroupIDs {
			ps, e := tx.GroupPackages(ctx, id)
			if e != nil || len(ps) == 0 {
				return ErrPackageDenied
			}
			for _, p := range ps {
				packages[p.ID] = p
			}
		}
		for _, p := range packages {
			product, e := tx.GetProduct(ctx, p.ProductID)
			if e != nil || product.Status != domain.ProductActive {
				return ErrProductDisabled
			}
			if e = tx.SaveGrant(ctx, domain.PackageGrant{AgentID: a.ID, PackageID: p.ID, Enabled: true}); e != nil {
				return e
			}
			if e = tx.EnsurePackagePolicy(ctx, a.ID, p, in.MaxBatchQuantity, in.CanExport); e != nil {
				return e
			}
		}
		if in.InitialBalance > 0 {
			if e := tx.InsertMoney(ctx, domain.MoneyLedger{ID: yncrypto.NewID("money"), AgentID: a.ID, RequestID: "open-" + a.ID, Kind: "adjust", Amount: in.InitialBalance, BalanceAfter: in.InitialBalance, Remark: strings.TrimSpace(in.Reason), ActorID: actor, CreatedAt: now}); e != nil {
				return e
			}
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "agent.open", map[string]any{"agent_id": a.ID, "user_id": c.ID, "tier_id": in.TierID, "initial_balance": in.InitialBalance, "package_ids": in.PackageIDs, "group_ids": in.GroupIDs})
	})
	if err != nil {
		return out, err
	}
	return OpenAgentResult{Agent: a, User: c, Account: account, LoginURL: "/agent-console/", UserURL: "/user-console/"}, nil
}

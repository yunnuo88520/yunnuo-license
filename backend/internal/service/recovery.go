package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/mailer"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

var ErrRecoveryInvalid = unauthorized("RECOVERY_INVALID", "recovery code is invalid, expired or already used")

func recoveryHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func canRecover(role, kind, targetRole string) bool {
	return role == domain.AdminRoleSuperAdmin || role == domain.AdminRoleAdmin && (kind != "admin" || targetRole == domain.AdminRoleDeveloper)
}

type RecoveryInput struct {
	Reason  string `json:"reason"`
	Minutes int    `json:"minutes"`
}
type RecoveryResult struct {
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expires_at"`
	RecoveryURL string    `json:"recovery_url"`
}

func (s *Service) RecoveryAccounts(ctx context.Context, kind, agentID, role string) ([]domain.RecoveryAccount, error) {
	items := []domain.RecoveryAccount{}
	switch kind {
	case "customer":
		rows, err := s.Customers(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			items = append(items, domain.RecoveryAccount{ID: r.ID, Username: r.Username, DisplayName: r.DisplayName, Role: "customer", Status: r.Status})
		}
	case "agent":
		rows, err := s.ListAgentUsers(ctx, agentID)
		if err != nil {
			return nil, err
		}
		// 账号未单独填邮箱时回退到代理联系邮箱，与恢复码邮件的实际投递地址保持一致。
		fallbackEmail := ""
		if agent, err := s.currentStore().GetAgent(ctx, agentID); err == nil {
			fallbackEmail = strings.ToLower(strings.TrimSpace(agent.Email))
		}
		for _, r := range rows {
			email := strings.ToLower(strings.TrimSpace(r.Email))
			if email == "" {
				email = fallbackEmail
			}
			items = append(items, domain.RecoveryAccount{ID: r.ID, Username: r.Username, DisplayName: r.DisplayName, Email: email, Role: r.Role, Status: r.Status})
		}
	case "admin":
		rows, err := s.ListAdminUsers(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if canRecover(role, kind, r.Role) {
				items = append(items, domain.RecoveryAccount{ID: r.ID, Username: r.Username, DisplayName: r.DisplayName, Role: r.Role, Status: r.Status})
			}
		}
	default:
		return nil, ErrInvalidRequest
	}
	return items, nil
}
func (s *Service) CreateRecovery(ctx context.Context, kind, id, actor, role string, in RecoveryInput) (RecoveryResult, error) {
	var out RecoveryResult
	if err := validateRecoveryInput(&in); err != nil {
		return out, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return out, err
	}
	out.Code = "reset_" + base64.RawURLEncoding.EncodeToString(raw)
	expiresAt, err := s.issuePasswordReset(ctx, kind, id, actor, role, in, recoveryHash(out.Code), true)
	if err != nil {
		return RecoveryResult{}, err
	}
	out.ExpiresAt = expiresAt
	out.RecoveryURL = "/recover"
	return out, nil
}

// CreateRecoveryByEmail 创建 6 位数字验证码并发送到账号邮箱。验证码使用账号作用域
// 哈希存储（防止不同账号撞码），兑换时需同时提供登录账号；验证码本身不回传接口。
func (s *Service) CreateRecoveryByEmail(ctx context.Context, kind, id, actor, role string, in RecoveryInput) (RecoveryEmailResult, error) {
	var out RecoveryEmailResult
	if err := validateRecoveryInput(&in); err != nil {
		return out, err
	}
	account, err := s.currentStore().RecoveryAccount(ctx, kind, id)
	if err != nil {
		return out, notFound("ACCOUNT_NOT_FOUND", "account not found")
	}
	email := strings.ToLower(strings.TrimSpace(account.Email))
	if email == "" {
		return out, badRequest("ACCOUNT_EMAIL_MISSING", "该账号没有可用的邮箱地址，请改用手动交付恢复码")
	}
	// 先确认邮件功能可用再签发，避免验证码已失效旧码却发不出去。
	r, err := s.emailRuntime(ctx)
	if err != nil {
		return out, err
	}
	if !r.usable() || mailer.Validate(r.cfg) != nil {
		return out, badRequest("EMAIL_NOT_ENABLED", "邮件功能未启用或 SMTP 配置不完整，请改用手动交付")
	}
	code, err := sixDigitCode()
	if err != nil {
		return out, err
	}
	scopeHash := recoveryHash(kind + "\x00" + id + "\x00" + code)
	expiresAt, err := s.issuePasswordReset(ctx, kind, id, actor, role, in, scopeHash, true)
	if err != nil {
		return out, err
	}
	if err := s.sendVerificationEmail(ctx, r, []string{email}, domain.EmailKindRecovery, "", verifyCodeEmail{
		Code: code, Purpose: "密码重置验证码",
		Greeting:     "您好 " + account.Username + "：",
		Intro:        "请在密码恢复页输入登录账号 " + account.Username + " 和下方 6 位验证码，即可设置新密码：",
		ValidMinutes: int(time.Until(expiresAt).Minutes()) + 1,
		ActionText:   "打开密码恢复页", ActionURL: recoverURL(r),
		Warning: "该验证码仅可使用一次；请勿透露给任何人，若非本人操作请尽快联系平台管理员。",
	}); err != nil {
		return out, badRequest("EMAIL_SEND_FAILED", "验证码已签发，但邮件发送失败: "+err.Error())
	}
	_ = s.audit(ctx, "admin", actor, "", "", "", "account.recovery.email", "success", "")
	out.SentTo = email
	out.ExpiresAt = expiresAt
	return out, nil
}

func validateRecoveryInput(in *RecoveryInput) error {
	if in.Minutes == 0 {
		in.Minutes = 30
	}
	if in.Minutes < 5 || in.Minutes > 1440 || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 {
		return ErrInvalidRequest
	}
	return nil
}

// issuePasswordReset 在事务中签发一次性恢复码：作废同账号此前未用的码并落库审计。
// privileged 标记管理员代发流程；自助流程跳过管理员权限校验（仅限绑定邮箱的账号）。
func (s *Service) issuePasswordReset(ctx context.Context, kind, id, actor, role string, in RecoveryInput, tokenHash string, privileged bool) (time.Time, error) {
	expiresAt := s.now().Add(time.Duration(in.Minutes) * time.Minute)
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAccount(ctx, kind, id); err != nil {
			return ErrInvalidRequest
		}
		a, err := tx.RecoveryAccount(ctx, kind, id)
		if err != nil {
			return notFound("ACCOUNT_NOT_FOUND", "account not found")
		}
		if privileged && !canRecover(role, kind, a.Role) {
			return ErrPermissionDenied
		}
		if a.Status != "active" {
			return forbidden("ACCOUNT_DISABLED", "account is disabled")
		}
		r := domain.PasswordReset{ID: yncrypto.NewID("reset"), TokenHash: tokenHash, Kind: kind, UserID: id, CredentialHash: recoveryHash(a.PasswordHash), ExpiresAt: expiresAt, CreatedBy: actor, Reason: in.Reason, CreatedAt: s.now()}
		if err := tx.CreatePasswordReset(ctx, r); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "account.recovery.issue", map[string]string{"account_kind": kind, "user_id": id, "reset_id": r.ID, "reason": in.Reason})
	})
	return expiresAt, err
}

func sixDigitCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// RequestRecoveryEmail 自助找回：账号持有人输入登录账号，验证码发送到绑定
// 邮箱（代理账号未单独绑定邮箱时回退代理联系邮箱）。为防枚举，无论账号是否存在、
// 是否绑定邮箱都返回相同的通用提示；同一账号 60 秒内只发一封，防止邮箱轰炸。
func (s *Service) RequestRecoveryEmail(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 80 {
		return ErrInvalidRequest
	}
	refs, err := s.currentStore().ResetAccountRefsByUsername(ctx, username)
	if err != nil {
		return err
	}
	var kind, id, email string
	for _, ref := range refs {
		if ref.Status != "active" {
			continue
		}
		account, err := s.currentStore().RecoveryAccount(ctx, ref.Kind, ref.ID)
		if err != nil {
			continue
		}
		if mail := strings.ToLower(strings.TrimSpace(account.Email)); mail != "" {
			kind, id, email = ref.Kind, ref.ID, mail
			break
		}
	}
	if kind == "" {
		return nil
	}
	// 冷却期内直接跳过发送（仍返回成功文案，不给探测空间）。
	if last, ok, err := s.currentStore().LastPasswordResetCreated(ctx, kind, id); err == nil && ok && s.now().Sub(last) < time.Minute {
		return nil
	}
	r, err := s.emailRuntime(ctx)
	if err != nil {
		return nil
	}
	if !r.usable() {
		return nil
	}
	in := RecoveryInput{Reason: "账号持有人自助找回密码", Minutes: 30}
	code, err := sixDigitCode()
	if err != nil {
		return nil
	}
	scopeHash := recoveryHash(kind + "\x00" + id + "\x00" + code)
	if _, err := s.issuePasswordReset(ctx, kind, id, id, "", in, scopeHash, false); err != nil {
		return nil
	}
	sendErr := s.sendVerificationEmail(ctx, r, []string{email}, domain.EmailKindRecovery, "", verifyCodeEmail{
		Code: code, Purpose: "密码重置验证码",
		Greeting:     "您好 " + username + "：",
		Intro:        "您正在自助重置密码，请在密码恢复页输入登录账号 " + username + " 和下方 6 位验证码：",
		ValidMinutes: 30, ActionText: "打开密码恢复页", ActionURL: recoverURL(r),
		Warning: "该验证码仅可使用一次；若非本人操作，请忽略此邮件并尽快联系平台管理员。",
	})
	if sendErr != nil {
		return nil
	}
	_ = s.audit(ctx, kind, id, "", "", "", "account.recovery.request", "success", "")
	return nil
}

// locateAccountReset 按登录账号定位 6 位验证码对应的恢复记录。代理账号的用户名
// 只在代理内唯一，因此逐一尝试同名账号，验证码哈希本身保证只命中正确账号。
func (s *Service) locateAccountReset(ctx context.Context, username, code string) (domain.PasswordReset, *domain.ResetAccountRef, error) {
	if username == "" {
		return domain.PasswordReset{}, nil, ErrRecoveryInvalid
	}
	refs, err := s.currentStore().ResetAccountRefsByUsername(ctx, username)
	if err != nil {
		return domain.PasswordReset{}, nil, err
	}
	for _, ref := range refs {
		r, err := s.currentStore().PasswordResetByAccount(ctx, ref.Kind, ref.ID, recoveryHash(ref.Kind+"\x00"+ref.ID+"\x00"+code))
		if err == nil {
			return r, &ref, nil
		}
		if !store.IsNotFound(err) {
			return domain.PasswordReset{}, nil, err
		}
	}
	return domain.PasswordReset{}, nil, ErrRecoveryInvalid
}

func recoverURL(r *emailRuntime) string {
	if r.settings.SiteURL == "" {
		return ""
	}
	return r.settings.SiteURL + "/recover"
}

// RedeemRecovery 支持两种恢复码：管理员手动交付的长恢复码（全局查找），以及
// 发送到邮箱的 6 位验证码（需提供登录账号，按账号作用域哈希查找）。
func (s *Service) RedeemRecovery(ctx context.Context, code, password, username string) error {
	if len(code) > 100 || len(password) < 8 || len(password) > 256 {
		return ErrRecoveryInvalid
	}
	code = strings.TrimSpace(code)
	tokenHash := recoveryHash(code)
	r, err := s.currentStore().PasswordReset(ctx, tokenHash)
	var scope *domain.ResetAccountRef
	if err != nil {
		if !store.IsNotFound(err) {
			return ErrRecoveryInvalid
		}
		if r, scope, err = s.locateAccountReset(ctx, strings.TrimSpace(username), code); err != nil {
			return ErrRecoveryInvalid
		}
	}
	hash, err := hashAgentPassword(password)
	if err != nil {
		return err
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAccount(ctx, r.Kind, r.UserID); err != nil {
			return err
		}
		var fresh domain.PasswordReset
		if scope != nil {
			fresh, err = tx.PasswordResetByAccount(ctx, scope.Kind, scope.ID, recoveryHash(scope.Kind+"\x00"+scope.ID+"\x00"+code))
		} else {
			fresh, err = tx.PasswordReset(ctx, tokenHash)
		}
		if err != nil || fresh.ConsumedAt != nil || !s.now().Before(fresh.ExpiresAt) {
			return ErrRecoveryInvalid
		}
		a, err := tx.RecoveryAccount(ctx, r.Kind, r.UserID)
		if err != nil || a.Status != "active" || recoveryHash(a.PasswordHash) != fresh.CredentialHash {
			return ErrRecoveryInvalid
		}
		if err := tx.ResetAccountPassword(ctx, r.Kind, r.UserID, hash, s.now()); err != nil {
			return err
		}
		if err := tx.ConsumePasswordReset(ctx, r.ID, s.now()); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, r.Kind, r.UserID, "", "", "account.recovery.redeem", map[string]string{"reset_id": r.ID})
	})
}

// Used by an authenticated administrator or the server operator's offline recovery command.
func (s *Service) ResetAccountPassword(ctx context.Context, kind, id, password, reason, actor, role string) error {
	if len(password) < 8 || len(password) > 256 || strings.TrimSpace(reason) == "" || len(reason) > 500 {
		return ErrInvalidRequest
	}
	hash, err := hashAgentPassword(password)
	if err != nil {
		return err
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAccount(ctx, kind, id); err != nil {
			return ErrInvalidRequest
		}
		a, err := tx.RecoveryAccount(ctx, kind, id)
		if err != nil {
			return notFound("ACCOUNT_NOT_FOUND", "account not found")
		}
		if !canRecover(role, kind, a.Role) {
			return ErrPermissionDenied
		}
		if err := tx.ResetAccountPassword(ctx, kind, id, hash, s.now()); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "account.password.reset", map[string]string{"account_kind": kind, "user_id": id, "reason": reason})
	})
}

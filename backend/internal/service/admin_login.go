package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

const adminLoginChallengeTTL = 5 * time.Minute

var (
	ErrMFALoginInvalid = unauthorized("MFA_CHALLENGE_INVALID", "登录验证已失效，请重新输入账号和密码")
	ErrMFALoginExpired = unauthorized("MFA_CHALLENGE_EXPIRED", "登录验证已过期，请重新输入账号和密码")
	ErrMFACodeInvalid  = forbidden("MFA_CODE_INVALID", "验证码无效或已使用，请检查后重试")
)

type AdminLoginChallenge struct {
	Challenge string    `json:"challenge"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AdminLoginStartResponse struct {
	Login *AdminLoginResponse  `json:"login,omitempty"`
	MFA   *AdminLoginChallenge `json:"mfa,omitempty"`
}

type CompleteAdminLoginInput struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}

// A successful OTP changes LastStep; a recovery code changes RecoveryHashes.
// Signing that state makes all challenges issued before the successful login
// unusable. Validation and the state change share the administrator row lock,
// including with legacy login, so concurrent completions cannot replay one.
func adminMFAState(m store.AdminMFA) string {
	raw, _ := json.Marshal([]any{m.Secret, m.LastStep, m.RecoveryHashes})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// StartAdminLogin verifies password and the optional image CAPTCHA. Only an
// account which already enabled two-factor authentication gets a second step.
func (s *Service) StartAdminLogin(ctx context.Context, input AdminLoginInput, clientIP string) (AdminLoginStartResponse, error) {
	user, err := s.checkAdminLoginCredentials(ctx, input)
	if err != nil {
		return AdminLoginStartResponse{}, err
	}
	var out AdminLoginStartResponse
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAdmin(ctx, user.ID); err != nil {
			return err
		}
		current, err := tx.GetAdminUserByID(ctx, user.ID)
		if err != nil {
			return err
		}
		if current.Status != domain.AdminUserActive || current.SessionVersion != user.SessionVersion {
			return ErrInvalidCredentials
		}
		m, err := tx.AdminMFA(ctx, user.ID)
		if store.IsNotFound(err) || (err == nil && !m.Enabled) {
			return nil
		}
		if err != nil {
			return err
		}
		now := s.now()
		expires := time.Unix(now.Add(adminLoginChallengeTTL).Unix(), 0).UTC()
		challenge, err := s.signAdminToken(adminTokenPayload{
			Type: "admin_mfa_challenge", UserID: user.ID, SessionVersion: user.SessionVersion,
			IssuedAt: now.Unix(), ExpiresAt: expires.Unix(), MFAState: adminMFAState(m), ClientIP: clientIP,
		})
		if err != nil {
			return err
		}
		out.MFA = &AdminLoginChallenge{Challenge: challenge, ExpiresAt: expires}
		return nil
	})
	if err != nil || out.MFA != nil {
		return out, err
	}
	login, err := s.issueAdminLogin(ctx, user)
	if err == nil {
		out.Login = &login
	}
	return out, err
}

// CompleteAdminLogin never accepts a password or reuses the one-time CAPTCHA.
// Its signed, short-lived proof is bound to this client IP, account session
// version, and the unused MFA state. It cannot authenticate ordinary API calls.
func (s *Service) CompleteAdminLogin(ctx context.Context, input CompleteAdminLoginInput, clientIP string) (AdminLoginResponse, error) {
	if len(input.Challenge) > 4096 {
		return AdminLoginResponse{}, ErrMFALoginInvalid
	}
	payload, err := s.verifyAdminToken(strings.TrimSpace(input.Challenge))
	if err != nil || payload.Type != "admin_mfa_challenge" || payload.UserID == "" || payload.MFAState == "" || payload.ClientIP != clientIP {
		return AdminLoginResponse{}, ErrMFALoginInvalid
	}
	if s.now().Unix() >= payload.ExpiresAt {
		return AdminLoginResponse{}, ErrMFALoginExpired
	}
	var user domain.AdminUser
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAdmin(ctx, payload.UserID); err != nil {
			return err
		}
		var err error
		user, err = tx.GetAdminUserByID(ctx, payload.UserID)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrMFALoginInvalid
			}
			return err
		}
		if user.Status != domain.AdminUserActive || user.SessionVersion != payload.SessionVersion {
			return ErrMFALoginInvalid
		}
		m, err := tx.AdminMFA(ctx, user.ID)
		if store.IsNotFound(err) {
			return ErrMFALoginInvalid
		}
		if err != nil {
			return err
		}
		if !m.Enabled || subtle.ConstantTimeCompare([]byte(adminMFAState(m)), []byte(payload.MFAState)) != 1 {
			return ErrMFALoginInvalid
		}
		if s.now().Unix() >= payload.ExpiresAt {
			return ErrMFALoginExpired
		}
		if !s.verifyMFACode(&m, input.Code, true) {
			return ErrMFACodeInvalid
		}
		return tx.SaveAdminMFA(ctx, m, false)
	})
	if err != nil {
		return AdminLoginResponse{}, err
	}
	return s.issueAdminLogin(ctx, user)
}

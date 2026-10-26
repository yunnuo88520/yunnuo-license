package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

const adminTokenTTL = 12 * time.Hour

type CreateAdminUserInput struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Email       string `json:"email"`
}

func (s *Service) EnsureBootstrapAdmin(ctx context.Context, username, password, displayName string) (domain.AdminUser, bool, error) {
	count, err := s.currentStore().CountAdminUsers(ctx)
	if err != nil {
		return domain.AdminUser{}, false, err
	}
	if count > 0 {
		return domain.AdminUser{}, false, nil
	}
	user, err := s.CreateAdminUser(ctx, CreateAdminUserInput{
		Username:    username,
		Password:    password,
		DisplayName: displayName,
		Role:        domain.AdminRoleSuperAdmin,
	})
	if err != nil {
		return domain.AdminUser{}, false, err
	}
	return user, true, nil
}

func (s *Service) CreateAdminUser(ctx context.Context, input CreateAdminUserInput) (domain.AdminUser, error) {
	input.Username = normalizeUsername(input.Username)
	input.Password = strings.TrimSpace(input.Password)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.Username == "" || len(input.Password) < 8 || !validAdminRole(input.Role) {
		return domain.AdminUser{}, ErrInvalidRequest
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return domain.AdminUser{}, err
	}
	if err := s.ensureEmailAvailable(ctx, "admin_users", email); err != nil {
		return domain.AdminUser{}, err
	}
	passwordHash, err := hashAgentPassword(input.Password)
	if err != nil {
		return domain.AdminUser{}, err
	}
	now := s.now()
	user := domain.AdminUser{
		ID:             yncrypto.NewID("admin"),
		Username:       input.Username,
		Email:          email,
		PasswordHash:   passwordHash,
		DisplayName:    input.DisplayName,
		Role:           input.Role,
		Status:         domain.AdminUserActive,
		SessionVersion: 1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.currentStore().CreateAdminUser(ctx, user); err != nil {
		return domain.AdminUser{}, err
	}
	_ = s.audit(ctx, "admin", "", "", "", "", "admin_user.create", "success", "")
	return user, nil
}

func (s *Service) ListAdminUsers(ctx context.Context) ([]domain.AdminUser, error) {
	return s.currentStore().ListAdminUsers(ctx)
}

type AdminLoginInput struct {
	OTPCode  string `json:"otp_code"`
	Username string `json:"username"`
	Password string `json:"password"`
	// 登录图形验证码：开关开启时必填（管理员/开发者/用户中心登录共用）。
	CaptchaID   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
}

type AdminLoginResponse struct {
	AccessToken string           `json:"access_token"`
	TokenType   string           `json:"token_type"`
	ExpiresAt   time.Time        `json:"expires_at"`
	User        domain.AdminUser `json:"user"`
}

type AdminSession struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	Role        string    `json:"role"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type adminTokenPayload struct {
	Type           string `json:"type"`
	UserID         string `json:"user_id"`
	SessionVersion int    `json:"session_version"`
	IssuedAt       int64  `json:"iat"`
	ExpiresAt      int64  `json:"exp"`
	MFAState       string `json:"mfa_state,omitempty"`
	ClientIP       string `json:"client_ip,omitempty"`
}

func (s *Service) AdminLogin(ctx context.Context, input AdminLoginInput) (AdminLoginResponse, error) {
	user, err := s.checkAdminLoginCredentials(ctx, input)
	if err != nil {
		return AdminLoginResponse{}, err
	}
	if err := s.verifyAdminMFA(ctx, user.ID, input.OTPCode); err != nil {
		return AdminLoginResponse{}, err
	}
	return s.issueAdminLogin(ctx, user)
}

func (s *Service) checkAdminLoginCredentials(ctx context.Context, input AdminLoginInput) (domain.AdminUser, error) {
	if err := s.ensureLoginCaptcha(ctx, input.CaptchaID, input.CaptchaCode); err != nil {
		return domain.AdminUser{}, err
	}
	input.Username = normalizeUsername(input.Username)
	if input.Username == "" || strings.TrimSpace(input.Password) == "" {
		return domain.AdminUser{}, ErrInvalidCredentials
	}
	user, err := s.currentStore().GetAdminUserByUsername(ctx, input.Username)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.AdminUser{}, ErrInvalidCredentials
		}
		return domain.AdminUser{}, err
	}
	if user.Status != domain.AdminUserActive {
		return domain.AdminUser{}, ErrAdminUserDisabled
	}
	ok, err := verifyAgentPassword(input.Password, user.PasswordHash)
	if err != nil || !ok {
		return domain.AdminUser{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) issueAdminLogin(ctx context.Context, user domain.AdminUser) (AdminLoginResponse, error) {
	now := s.now()
	expiresAt := now.Add(adminTokenTTL)
	token, err := s.signAdminToken(adminTokenPayload{
		Type:           "admin_session",
		UserID:         user.ID,
		SessionVersion: user.SessionVersion,
		IssuedAt:       now.Unix(),
		ExpiresAt:      expiresAt.Unix(),
	})
	if err != nil {
		return AdminLoginResponse{}, err
	}
	if err := s.currentStore().UpdateAdminUserLastLogin(ctx, user.ID, now); err != nil {
		return AdminLoginResponse{}, err
	}
	user.LastLoginAt = &now
	user.UpdatedAt = now
	_ = s.audit(ctx, "admin", user.ID, "", "", "", "admin.login", "success", "")
	return AdminLoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		User:        user,
	}, nil
}

func (s *Service) AuthenticateAdminToken(ctx context.Context, token string) (AdminSession, error) {
	payload, err := s.verifyAdminToken(strings.TrimSpace(token))
	if err != nil {
		return AdminSession{}, ErrInvalidAdminToken
	}
	now := s.now()
	if payload.Type != "admin_session" || payload.UserID == "" || now.Unix() >= payload.ExpiresAt {
		return AdminSession{}, ErrInvalidAdminToken
	}
	user, err := s.currentStore().GetAdminUserByID(ctx, payload.UserID)
	if err != nil {
		if store.IsNotFound(err) {
			return AdminSession{}, ErrInvalidAdminToken
		}
		return AdminSession{}, err
	}
	if user.Status != domain.AdminUserActive {
		return AdminSession{}, ErrAdminUserDisabled
	}
	if user.SessionVersion != payload.SessionVersion || !validAdminRole(user.Role) {
		return AdminSession{}, ErrInvalidAdminToken
	}
	return AdminSession{
		UserID:      user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		ExpiresAt:   time.Unix(payload.ExpiresAt, 0).UTC(),
	}, nil
}

type ChangeAdminPasswordInput struct {
	CurrentPassword string  `json:"current_password"`
	NewPassword     string  `json:"new_password"`
	ConfirmPassword *string `json:"confirm_password,omitempty"`
}

func (s *Service) ChangeAdminPassword(ctx context.Context, userID string, input ChangeAdminPasswordInput) error {
	if userID == "" {
		return ErrInvalidRequest
	}
	if err := validatePasswordChange(input); err != nil {
		return err
	}
	user, err := s.currentStore().GetAdminUserByID(ctx, userID)
	if err != nil {
		if store.IsNotFound(err) {
			return ErrInvalidAdminToken
		}
		return err
	}
	ok, err := verifyAgentPassword(input.CurrentPassword, user.PasswordHash)
	if err != nil || !ok {
		return ErrCurrentPasswordIncorrect
	}
	passwordHash, err := hashAgentPassword(input.NewPassword)
	if err != nil {
		return err
	}
	if err := s.currentStore().UpdateAdminUserPassword(ctx, user.ID, passwordHash, s.now()); err != nil {
		return err
	}
	_ = s.audit(ctx, "admin", user.ID, "", "", "", "admin.password.change", "success", "")
	return nil
}

func (s *Service) signAdminToken(payload adminTokenPayload) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encodedBody := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, s.adminTokenKey())
	mac.Write([]byte(encodedBody))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encodedBody + "." + signature, nil
}

func (s *Service) verifyAdminToken(token string) (adminTokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return adminTokenPayload{}, ErrInvalidAdminToken
	}
	mac := hmac.New(sha256.New, s.adminTokenKey())
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || subtle.ConstantTimeCompare(signature, expected) != 1 {
		return adminTokenPayload{}, ErrInvalidAdminToken
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return adminTokenPayload{}, ErrInvalidAdminToken
	}
	var payload adminTokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return adminTokenPayload{}, ErrInvalidAdminToken
	}
	return payload, nil
}

func (s *Service) adminTokenKey() []byte {
	mac := hmac.New(sha256.New, s.cardPepper)
	mac.Write([]byte("yn-license-admin-session-v1"))
	return mac.Sum(nil)
}

func validAdminRole(role string) bool {
	switch role {
	case domain.AdminRoleSuperAdmin, domain.AdminRoleAdmin, domain.AdminRoleOperator, domain.AdminRoleAuditor, domain.AdminRoleDeveloper:
		return true
	default:
		return false
	}
}

type UpdateMyEmailInput struct {
	Email   string `json:"email"`
	ActorID string `json:"-"`
}

// UpdateMyEmail 管理员自助更新自己的邮箱（找回密码与通知依赖该邮箱）。
func (s *Service) UpdateMyEmail(ctx context.Context, actorID string, input UpdateMyEmailInput) (domain.AdminUser, error) {
	if strings.TrimSpace(actorID) == "" {
		return domain.AdminUser{}, ErrPermissionDenied
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return domain.AdminUser{}, err
	}
	if email != "" {
		owner, err := s.currentStore().AdminIDByEmail(ctx, email)
		if err != nil {
			return domain.AdminUser{}, err
		}
		if owner != "" && owner != actorID {
			return domain.AdminUser{}, badRequest("EMAIL_TAKEN", "该邮箱已被其他账号使用")
		}
	}
	if err := s.currentStore().UpdateAdminUserEmail(ctx, actorID, email); err != nil {
		return domain.AdminUser{}, err
	}
	_ = s.audit(ctx, "admin", actorID, "", "", "", "admin.email.update", "success", "")
	return s.currentStore().GetAdminUserByID(ctx, actorID)
}

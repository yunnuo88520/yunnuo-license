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
}

func (s *Service) EnsureBootstrapAdmin(ctx context.Context, username, password, displayName string) (domain.AdminUser, bool, error) {
	count, err := s.store.CountAdminUsers(ctx)
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
	passwordHash, err := hashAgentPassword(input.Password)
	if err != nil {
		return domain.AdminUser{}, err
	}
	now := s.now()
	user := domain.AdminUser{
		ID:             yncrypto.NewID("admin"),
		Username:       input.Username,
		PasswordHash:   passwordHash,
		DisplayName:    input.DisplayName,
		Role:           input.Role,
		Status:         domain.AdminUserActive,
		SessionVersion: 1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.store.CreateAdminUser(ctx, user); err != nil {
		return domain.AdminUser{}, err
	}
	_ = s.audit(ctx, "admin", "", "", "", "", "admin_user.create", "success", "")
	return user, nil
}

func (s *Service) ListAdminUsers(ctx context.Context) ([]domain.AdminUser, error) {
	return s.store.ListAdminUsers(ctx)
}

type AdminLoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
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
}

func (s *Service) AdminLogin(ctx context.Context, input AdminLoginInput) (AdminLoginResponse, error) {
	input.Username = normalizeUsername(input.Username)
	if input.Username == "" || strings.TrimSpace(input.Password) == "" {
		return AdminLoginResponse{}, ErrInvalidCredentials
	}
	user, err := s.store.GetAdminUserByUsername(ctx, input.Username)
	if err != nil {
		if store.IsNotFound(err) {
			return AdminLoginResponse{}, ErrInvalidCredentials
		}
		return AdminLoginResponse{}, err
	}
	if user.Status != domain.AdminUserActive {
		return AdminLoginResponse{}, ErrAdminUserDisabled
	}
	ok, err := verifyAgentPassword(input.Password, user.PasswordHash)
	if err != nil || !ok {
		return AdminLoginResponse{}, ErrInvalidCredentials
	}
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
	if err := s.store.UpdateAdminUserLastLogin(ctx, user.ID, now); err != nil {
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
	user, err := s.store.GetAdminUserByID(ctx, payload.UserID)
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
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Service) ChangeAdminPassword(ctx context.Context, userID string, input ChangeAdminPasswordInput) error {
	if userID == "" || strings.TrimSpace(input.CurrentPassword) == "" || len(strings.TrimSpace(input.NewPassword)) < 8 {
		return ErrInvalidRequest
	}
	user, err := s.store.GetAdminUserByID(ctx, userID)
	if err != nil {
		if store.IsNotFound(err) {
			return ErrInvalidAdminToken
		}
		return err
	}
	ok, err := verifyAgentPassword(input.CurrentPassword, user.PasswordHash)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	passwordHash, err := hashAgentPassword(strings.TrimSpace(input.NewPassword))
	if err != nil {
		return err
	}
	if err := s.store.UpdateAdminUserPassword(ctx, user.ID, passwordHash, s.now()); err != nil {
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
	case domain.AdminRoleSuperAdmin, domain.AdminRoleAdmin, domain.AdminRoleOperator, domain.AdminRoleAuditor:
		return true
	default:
		return false
	}
}

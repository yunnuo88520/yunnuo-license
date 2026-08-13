package service

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

const (
	agentPasswordHashVersion = "pbkdf2_sha256_v1"
	agentPasswordIterations  = 210000
	agentPasswordSaltBytes   = 16
	agentPasswordKeyBytes    = 32
	agentTokenTTL            = 24 * time.Hour
)

type CreateAgentUserInput struct {
	AgentID     string `json:"agent_id"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Role        string `json:"role"`
}

func (s *Service) CreateAgentUser(ctx context.Context, input CreateAgentUserInput) (domain.AgentUser, error) {
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.Username = normalizeUsername(input.Username)
	input.Password = strings.TrimSpace(input.Password)
	if input.AgentID == "" || input.Username == "" || len(input.Password) < 6 {
		return domain.AgentUser{}, ErrInvalidRequest
	}
	if input.Role == "" {
		input.Role = domain.AgentRoleOwner
	}
	if !validAgentRole(input.Role) {
		return domain.AgentUser{}, ErrInvalidRequest
	}
	agent, err := s.store.GetAgent(ctx, input.AgentID)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.AgentUser{}, ErrAgentNotFound
		}
		return domain.AgentUser{}, err
	}
	if err := ensureAgentActive(agent); err != nil {
		return domain.AgentUser{}, err
	}
	passwordHash, err := hashAgentPassword(input.Password)
	if err != nil {
		return domain.AgentUser{}, err
	}
	now := s.now()
	user := domain.AgentUser{
		ID:           yncrypto.NewID("aguser"),
		AgentID:      input.AgentID,
		Username:     input.Username,
		PasswordHash: passwordHash,
		DisplayName:  strings.TrimSpace(input.DisplayName),
		Phone:        strings.TrimSpace(input.Phone),
		Email:        strings.TrimSpace(input.Email),
		Role:         input.Role,
		Status:       domain.AgentUserActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.CreateAgentUser(ctx, user); err != nil {
		return domain.AgentUser{}, err
	}
	_ = s.audit(ctx, "admin", "", "", "", "", "agent_user.create", "success", "")
	return user, nil
}

func (s *Service) ListAgentUsers(ctx context.Context, agentID string) ([]domain.AgentUser, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, ErrInvalidRequest
	}
	return s.store.ListAgentUsers(ctx, agentID)
}

type AgentLoginInput struct {
	AgentID   string `json:"agent_id"`
	AgentNo   string `json:"agent_no"`
	LoginCode string `json:"login_code"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

type AgentLoginResponse struct {
	AccessToken string           `json:"access_token"`
	TokenType   string           `json:"token_type"`
	ExpiresAt   time.Time        `json:"expires_at"`
	Agent       domain.Agent     `json:"agent"`
	User        domain.AgentUser `json:"user"`
}

type AgentSession struct {
	AgentID     string    `json:"agent_id"`
	AgentNo     string    `json:"agent_no"`
	LoginCode   string    `json:"login_code"`
	AgentName   string    `json:"agent_name"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	Role        string    `json:"role"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type agentTokenPayload struct {
	Type      string `json:"type"`
	AgentID   string `json:"agent_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func (s *Service) AgentLogin(ctx context.Context, input AgentLoginInput) (AgentLoginResponse, error) {
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.AgentNo = strings.ToUpper(strings.TrimSpace(input.AgentNo))
	input.LoginCode = normalizeAgentLoginCode(input.LoginCode)
	input.Username = normalizeUsername(input.Username)
	if input.Username == "" || strings.TrimSpace(input.Password) == "" || (input.AgentID == "" && input.AgentNo == "" && input.LoginCode == "") {
		return AgentLoginResponse{}, ErrInvalidCredentials
	}
	agent, err := s.agentForLogin(ctx, input.AgentID, input.LoginCode, input.AgentNo)
	if err != nil {
		return AgentLoginResponse{}, err
	}
	if err := ensureAgentActive(agent); err != nil {
		return AgentLoginResponse{}, err
	}
	user, err := s.store.GetAgentUserByUsername(ctx, agent.ID, input.Username)
	if err != nil {
		if store.IsNotFound(err) {
			return AgentLoginResponse{}, ErrInvalidCredentials
		}
		return AgentLoginResponse{}, err
	}
	if user.Status != domain.AgentUserActive {
		return AgentLoginResponse{}, ErrAgentUserDisabled
	}
	ok, err := verifyAgentPassword(input.Password, user.PasswordHash)
	if err != nil || !ok {
		return AgentLoginResponse{}, ErrInvalidCredentials
	}
	now := s.now()
	expiresAt := now.Add(agentTokenTTL)
	token, err := s.signAgentToken(agentTokenPayload{
		Type:      "agent_session",
		AgentID:   agent.ID,
		UserID:    user.ID,
		Username:  user.Username,
		Role:      user.Role,
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		return AgentLoginResponse{}, err
	}
	if err := s.store.UpdateAgentUserLastLogin(ctx, user.ID, now); err != nil {
		return AgentLoginResponse{}, err
	}
	user.LastLoginAt = &now
	user.UpdatedAt = now
	_ = s.auditAgent(ctx, user.ID, agent.ID, "", "agent.login", "success", "")
	return AgentLoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		Agent:       agent,
		User:        user,
	}, nil
}

func (s *Service) AuthenticateAgentToken(ctx context.Context, token string) (AgentSession, error) {
	payload, err := s.verifyAgentToken(strings.TrimSpace(token))
	if err != nil {
		return AgentSession{}, ErrInvalidAgentToken
	}
	now := s.now()
	if payload.Type != "agent_session" || payload.AgentID == "" || payload.UserID == "" || now.Unix() >= payload.ExpiresAt {
		return AgentSession{}, ErrInvalidAgentToken
	}
	agent, err := s.store.GetAgent(ctx, payload.AgentID)
	if err != nil {
		if store.IsNotFound(err) {
			return AgentSession{}, ErrInvalidAgentToken
		}
		return AgentSession{}, err
	}
	if err := ensureAgentActive(agent); err != nil {
		return AgentSession{}, err
	}
	loginCode, err := s.store.GetAgentLoginCode(ctx, agent.ID)
	if err != nil {
		if store.IsNotFound(err) {
			return AgentSession{}, ErrInvalidAgentToken
		}
		return AgentSession{}, err
	}
	agent.LoginCode = loginCode
	user, err := s.store.GetAgentUserByID(ctx, payload.UserID)
	if err != nil {
		if store.IsNotFound(err) {
			return AgentSession{}, ErrInvalidAgentToken
		}
		return AgentSession{}, err
	}
	if user.AgentID != payload.AgentID || user.Status != domain.AgentUserActive {
		return AgentSession{}, ErrAgentUserDisabled
	}
	return AgentSession{
		AgentID:     payload.AgentID,
		AgentNo:     agent.AgentNo,
		LoginCode:   agent.LoginCode,
		AgentName:   agent.Name,
		UserID:      payload.UserID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		ExpiresAt:   time.Unix(payload.ExpiresAt, 0).UTC(),
	}, nil
}

func (s *Service) agentForLogin(ctx context.Context, agentID, loginCode, agentNo string) (domain.Agent, error) {
	if loginCode != "" {
		agent, err := s.store.GetAgentByLoginCode(ctx, loginCode)
		if err != nil {
			if store.IsNotFound(err) {
				return domain.Agent{}, ErrInvalidCredentials
			}
			return domain.Agent{}, err
		}
		return agent, nil
	}
	if agentID != "" {
		agent, err := s.store.GetAgent(ctx, agentID)
		if err != nil {
			if store.IsNotFound(err) {
				return domain.Agent{}, ErrInvalidCredentials
			}
			return domain.Agent{}, err
		}
		return s.withAgentLoginCode(ctx, agent)
	}
	agent, err := s.store.GetAgentByNo(ctx, agentNo)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.Agent{}, ErrInvalidCredentials
		}
		return domain.Agent{}, err
	}
	return s.withAgentLoginCode(ctx, agent)
}

func (s *Service) withAgentLoginCode(ctx context.Context, agent domain.Agent) (domain.Agent, error) {
	loginCode, err := s.store.GetAgentLoginCode(ctx, agent.ID)
	if err != nil {
		return domain.Agent{}, err
	}
	agent.LoginCode = loginCode
	return agent, nil
}

func (s *Service) signAgentToken(payload agentTokenPayload) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encodedBody := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, s.agentTokenKey())
	mac.Write([]byte(encodedBody))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encodedBody + "." + signature, nil
}

func (s *Service) verifyAgentToken(token string) (agentTokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return agentTokenPayload{}, ErrInvalidAgentToken
	}
	mac := hmac.New(sha256.New, s.agentTokenKey())
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || subtle.ConstantTimeCompare(signature, expected) != 1 {
		return agentTokenPayload{}, ErrInvalidAgentToken
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return agentTokenPayload{}, ErrInvalidAgentToken
	}
	var payload agentTokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return agentTokenPayload{}, ErrInvalidAgentToken
	}
	return payload, nil
}

func (s *Service) agentTokenKey() []byte {
	mac := hmac.New(sha256.New, s.cardPepper)
	mac.Write([]byte("yn-license-agent-session-v1"))
	return mac.Sum(nil)
}

func hashAgentPassword(password string) (string, error) {
	salt := make([]byte, agentPasswordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, agentPasswordIterations, agentPasswordKeyBytes)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		agentPasswordHashVersion,
		strconv.Itoa(agentPasswordIterations),
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(key),
	}, "$"), nil
}

func verifyAgentPassword(password, stored string) (bool, error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != agentPasswordHashVersion {
		return false, fmt.Errorf("unsupported password hash")
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false, fmt.Errorf("invalid password iterations")
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false, err
	}
	expected, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return false, err
	}
	actual, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func normalizeAgentLoginCode(loginCode string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(loginCode), " ", ""))
}

func validAgentRole(role string) bool {
	switch role {
	case domain.AgentRoleOwner, domain.AgentRoleManager, domain.AgentRoleStaff, domain.AgentRoleReadonly:
		return true
	default:
		return false
	}
}

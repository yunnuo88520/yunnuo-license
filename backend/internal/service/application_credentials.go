package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

var (
	ErrApplicationCredentials = unauthorized("INVALID_APP_CREDENTIALS", "AppID or AppSecret is invalid")
	ErrApplicationToken       = unauthorized("INVALID_APP_ACCESS_TOKEN", "application access token is invalid or expired")
)

type ApplicationCredentialStatus struct {
	AppID      string     `json:"appid"`
	Configured bool       `json:"configured"`
	Version    int        `json:"version"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

func applicationSecretHash(secret string) string {
	hash := sha256.Sum256([]byte("yunnuo-app-secret-v1\x00" + secret))
	return hex.EncodeToString(hash[:])
}

func (s *Service) ApplicationCredentialStatus(ctx context.Context, id string) (ApplicationCredentialStatus, error) {
	p, err := s.IntegrationProduct(ctx, id)
	if err != nil {
		return ApplicationCredentialStatus{}, err
	}
	status := ApplicationCredentialStatus{AppID: p.ID}
	value, err := s.currentStore().ApplicationCredential(ctx, p.ID)
	if store.IsNotFound(err) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Configured = true
	status.Version = value.Version
	status.UpdatedAt = &value.UpdatedAt
	return status, nil
}

type ResetApplicationSecretInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason"`
}
type ApplicationSecretResponse struct {
	AppID     string `json:"appid"`
	AppSecret string `json:"app_secret"`
	Version   int    `json:"version"`
}

func (s *Service) ResetApplicationSecret(ctx context.Context, id, actor string, in ResetApplicationSecretInput) (ApplicationSecretResponse, error) {
	if in.ExpectedVersion < 0 || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 {
		return ApplicationSecretResponse{}, ErrInvalidRequest
	}
	if _, err := s.IntegrationProduct(ctx, id); err != nil {
		return ApplicationSecretResponse{}, err
	}
	secret, err := yncrypto.ApplicationSecret()
	if err != nil {
		return ApplicationSecretResponse{}, err
	}
	result := ApplicationSecretResponse{AppID: id, AppSecret: secret}
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		version, ok, e := tx.SetApplicationSecret(ctx, id, applicationSecretHash(secret), in.ExpectedVersion, s.now())
		if e != nil {
			return e
		}
		if !ok {
			return AppError{Code: "APP_SECRET_CHANGED", Message: "AppSecret was changed; refresh before resetting", HTTPStatus: 409}
		}
		result.Version = version
		kind := "admin"
		if a := ActorFrom(ctx); a != nil && a.Type == "developer" {
			kind = "developer"
		}
		return s.transactionAudit(ctx, tx, kind, actor, id, "", "application.secret.reset", map[string]any{"version": version, "reason": strings.TrimSpace(in.Reason)})
	})
	return result, err
}

type ApplicationTokenInput struct {
	GrantType string `json:"grant_type"`
	AppID     string `json:"appid"`
	Secret    string `json:"secret"`
}
type ApplicationTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}
type applicationTokenClaims struct {
	AppID     string `json:"appid"`
	Version   int    `json:"version"`
	ExpiresAt int64  `json:"exp"`
}

func (s *Service) ApplicationToken(ctx context.Context, in ApplicationTokenInput) (ApplicationTokenResponse, error) {
	if in.GrantType != "client_credential" {
		return ApplicationTokenResponse{}, ErrInvalidRequest
	}
	if len(in.Secret) != 32 || len(in.AppID) > 64 {
		return ApplicationTokenResponse{}, ErrApplicationCredentials
	}
	p, err := s.currentStore().GetProduct(ctx, strings.TrimSpace(in.AppID))
	if err != nil || p.Status != domain.ProductActive {
		return ApplicationTokenResponse{}, ErrApplicationCredentials
	}
	credential, err := s.currentStore().ApplicationCredential(ctx, p.ID)
	if err != nil || subtle.ConstantTimeCompare([]byte(credential.SecretHash), []byte(applicationSecretHash(in.Secret))) != 1 {
		return ApplicationTokenResponse{}, ErrApplicationCredentials
	}
	claims := applicationTokenClaims{AppID: p.ID, Version: credential.Version, ExpiresAt: s.now().Add(2 * time.Hour).Unix()}
	raw, err := json.Marshal(claims)
	if err != nil {
		return ApplicationTokenResponse{}, err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.dataKey)
	mac.Write([]byte("application-access-v1." + payload))
	token := "app1." + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if a := ActorFrom(ctx); a != nil {
		a.Type = "application"
		a.ID = p.ID
		a.Role = "application"
	}
	return ApplicationTokenResponse{AccessToken: token, TokenType: "Bearer", ExpiresIn: 7200}, nil
}

func (s *Service) AuthenticateApplicationToken(ctx context.Context, token string) (domain.Product, error) {
	if len(token) > 2048 {
		return domain.Product{}, ErrApplicationToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "app1" {
		return domain.Product{}, ErrApplicationToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return domain.Product{}, ErrApplicationToken
	}
	mac := hmac.New(sha256.New, s.dataKey)
	mac.Write([]byte("application-access-v1." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return domain.Product{}, ErrApplicationToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return domain.Product{}, ErrApplicationToken
	}
	var claims applicationTokenClaims
	if json.Unmarshal(raw, &claims) != nil || claims.AppID == "" || s.now().Unix() >= claims.ExpiresAt {
		return domain.Product{}, ErrApplicationToken
	}
	p, err := s.currentStore().GetProduct(ctx, claims.AppID)
	if err != nil || p.Status != domain.ProductActive {
		return domain.Product{}, ErrApplicationToken
	}
	credential, err := s.currentStore().ApplicationCredential(ctx, p.ID)
	if err != nil || credential.Version != claims.Version {
		return domain.Product{}, ErrApplicationToken
	}
	if a := ActorFrom(ctx); a != nil {
		a.Type = "application"
		a.ID = p.ID
		a.Role = "application"
	}
	return p, nil
}

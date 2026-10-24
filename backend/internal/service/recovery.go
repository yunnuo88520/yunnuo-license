package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"strings"
	"time"
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
		for _, r := range rows {
			items = append(items, domain.RecoveryAccount{ID: r.ID, Username: r.Username, DisplayName: r.DisplayName, Role: r.Role, Status: r.Status})
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
	if in.Minutes == 0 {
		in.Minutes = 30
	}
	if in.Minutes < 5 || in.Minutes > 1440 || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 {
		return out, ErrInvalidRequest
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return out, err
	}
	out.Code = "reset_" + base64.RawURLEncoding.EncodeToString(raw)
	out.ExpiresAt = s.now().Add(time.Duration(in.Minutes) * time.Minute)
	out.RecoveryURL = "/recover"
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
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
		if a.Status != "active" {
			return forbidden("ACCOUNT_DISABLED", "account is disabled")
		}
		r := domain.PasswordReset{ID: yncrypto.NewID("reset"), TokenHash: recoveryHash(out.Code), Kind: kind, UserID: id, CredentialHash: recoveryHash(a.PasswordHash), ExpiresAt: out.ExpiresAt, CreatedBy: actor, Reason: in.Reason, CreatedAt: s.now()}
		if err := tx.CreatePasswordReset(ctx, r); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "account.recovery.issue", map[string]string{"account_kind": kind, "user_id": id, "reset_id": r.ID, "reason": in.Reason})
	})
	if err != nil {
		return RecoveryResult{}, err
	}
	return out, nil
}
func (s *Service) RedeemRecovery(ctx context.Context, code, password string) error {
	if len(code) > 100 || len(password) < 8 || len(password) > 256 {
		return ErrRecoveryInvalid
	}
	tokenHash := recoveryHash(strings.TrimSpace(code))
	r, err := s.currentStore().PasswordReset(ctx, tokenHash)
	if err != nil {
		return ErrRecoveryInvalid
	}
	hash, err := hashAgentPassword(password)
	if err != nil {
		return err
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAccount(ctx, r.Kind, r.UserID); err != nil {
			return err
		}
		fresh, err := tx.PasswordReset(ctx, tokenHash)
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

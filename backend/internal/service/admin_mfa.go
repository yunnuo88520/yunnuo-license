package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"net/url"
	"strings"
	"time"
)

var ErrMFARequired = AppError{Code: "MFA_REQUIRED", Message: "请输入动态验证码或一次性恢复码", HTTPStatus: 403}

func totp(secret string, step int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	n := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1000000)
}
func (s *Service) verifyMFACode(m *store.AdminMFA, code string, allowRecovery bool) bool {
	secret, err := yncrypto.DecryptString(s.dataKey, m.Secret)
	if err != nil {
		return false
	}
	code = strings.TrimSpace(code)
	step := s.now().Unix() / 30
	if len(code) == 6 {
		for _, candidate := range []int64{step, step - 1, step + 1} {
			if candidate > m.LastStep && subtle.ConstantTimeCompare([]byte(totp(secret, candidate)), []byte(code)) == 1 {
				m.LastStep = candidate
				return true
			}
		}
	}
	if allowRecovery {
		var hashes []string
		if json.Unmarshal([]byte(m.RecoveryHashes), &hashes) != nil {
			return false
		}
		hash := sha256.Sum256([]byte(strings.ToLower(code)))
		want := hex.EncodeToString(hash[:])
		for i, stored := range hashes {
			if subtle.ConstantTimeCompare([]byte(want), []byte(stored)) == 1 {
				hashes = append(hashes[:i], hashes[i+1:]...)
				raw, _ := json.Marshal(hashes)
				m.RecoveryHashes = string(raw)
				return true
			}
		}
	}
	return false
}
func (s *Service) MFAStatus(ctx context.Context, id string) (map[string]any, error) {
	m, err := s.currentStore().AdminMFA(ctx, id)
	if store.IsNotFound(err) {
		return map[string]any{"enabled": false, "recovery_remaining": 0}, nil
	}
	if err != nil {
		return nil, err
	}
	var hashes []string
	json.Unmarshal([]byte(m.RecoveryHashes), &hashes)
	return map[string]any{"enabled": m.Enabled, "recovery_remaining": len(hashes)}, nil
}
func (s *Service) checkAdminPassword(ctx context.Context, id, password string) error {
	u, err := s.currentStore().GetAdminUserByID(ctx, id)
	if err != nil {
		return err
	}
	ok, err := verifyAgentPassword(password, u.PasswordHash)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	return nil
}
func (s *Service) BeginAdminMFA(ctx context.Context, id, password string) (map[string]string, error) {
	if err := s.checkAdminPassword(ctx, id, password); err != nil {
		return nil, err
	}
	var raw [20]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:])
	enc, err := yncrypto.EncryptString(s.dataKey, secret)
	if err != nil {
		return nil, err
	}
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAdmin(ctx, id); err != nil {
			return err
		}
		m, e := tx.AdminMFA(ctx, id)
		if e != nil && !store.IsNotFound(e) {
			return e
		}
		if m.Enabled {
			return badRequest("MFA_ALREADY_ENABLED", "动态验证已启用")
		}
		m = store.AdminMFA{UserID: id, Secret: enc, PendingAt: s.now().UTC().Format(time.RFC3339Nano), LastStep: -1, RecoveryHashes: "[]"}
		if e = tx.SaveAdminMFA(ctx, m, false); e != nil {
			return e
		}
		return s.transactionAudit(ctx, tx, "admin", id, "", "", "admin.mfa.begin", nil)
	})
	if err != nil {
		return nil, err
	}
	u, err := s.currentStore().GetAdminUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]string{"secret": secret, "otpauth_url": "otpauth://totp/" + url.PathEscape("Yunnuo:"+u.Username) + "?secret=" + secret + "&issuer=Yunnuo&algorithm=SHA1&digits=6&period=30"}, nil
}
func (s *Service) ConfirmAdminMFA(ctx context.Context, id, password, code string) ([]string, error) {
	if err := s.checkAdminPassword(ctx, id, password); err != nil {
		return nil, err
	}
	var codes, hashes []string
	for i := 0; i < 10; i++ {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return nil, err
		}
		value := hex.EncodeToString(raw[:])
		codes = append(codes, value)
		hash := sha256.Sum256([]byte(value))
		hashes = append(hashes, hex.EncodeToString(hash[:]))
	}
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAdmin(ctx, id); err != nil {
			return err
		}
		m, err := tx.AdminMFA(ctx, id)
		if err != nil {
			return ErrMFARequired
		}
		pending, e := time.Parse(time.RFC3339Nano, m.PendingAt)
		if e != nil || s.now().Sub(pending) > 10*time.Minute || m.Enabled || !s.verifyMFACode(&m, code, false) {
			return ErrMFARequired
		}
		m.Enabled = true
		encoded, _ := json.Marshal(hashes)
		m.RecoveryHashes = string(encoded)
		if err = tx.SaveAdminMFA(ctx, m, true); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", id, "", "", "admin.mfa.enable", nil)
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}
func (s *Service) DisableAdminMFA(ctx context.Context, id, password, code string) error {
	if err := s.checkAdminPassword(ctx, id, password); err != nil {
		return err
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAdmin(ctx, id); err != nil {
			return err
		}
		m, err := tx.AdminMFA(ctx, id)
		if err != nil || !m.Enabled || !s.verifyMFACode(&m, code, true) {
			return ErrMFARequired
		}
		m.Enabled = false
		m.Secret = ""
		m.RecoveryHashes = "[]"
		if err = tx.SaveAdminMFA(ctx, m, true); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", id, "", "", "admin.mfa.disable", nil)
	})
}
func (s *Service) verifyAdminMFA(ctx context.Context, id, code string) error {
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAdmin(ctx, id); err != nil {
			return err
		}
		m, err := tx.AdminMFA(ctx, id)
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !m.Enabled {
			return nil
		}
		if !s.verifyMFACode(&m, code, true) {
			return ErrMFARequired
		}
		return tx.SaveAdminMFA(ctx, m, false)
	})
}

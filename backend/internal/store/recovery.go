package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"time"
)

func accountTable(kind string) (string, error) {
	switch kind {
	case "customer":
		return "customer_users", nil
	case "agent":
		return "agent_users", nil
	case "admin":
		return "admin_users", nil
	}
	return "", fmt.Errorf("invalid account kind")
}
func readRecoveryAccount(ctx context.Context, q commerceQuery, kind, id string) (domain.RecoveryAccount, error) {
	var a domain.RecoveryAccount
	table, err := accountTable(kind)
	if err != nil {
		return a, err
	}
	role := "role"
	if kind == "customer" {
		role = "'customer'"
	}
	email := "''"
	if kind == "agent" {
		// 优先账号自身邮箱，其次回退到所属代理的联系邮箱。
		email = "COALESCE(NULLIF(email,''),(SELECT a.email FROM agents a WHERE a.id = agent_id),'')"
	}
	var name, mail sql.NullString
	err = q.QueryRowContext(ctx, `SELECT id,username,display_name,`+role+`,status,password_hash,`+email+` FROM `+table+` WHERE id=?`, id).Scan(&a.ID, &a.Username, &name, &a.Role, &a.Status, &a.PasswordHash, &mail)
	a.DisplayName = str(name)
	a.Email = mail.String
	return a, err
}
func (s *Store) RecoveryAccount(ctx context.Context, kind, id string) (domain.RecoveryAccount, error) {
	return readRecoveryAccount(ctx, s.db, kind, id)
}
func (tx *Tx) RecoveryAccount(ctx context.Context, kind, id string) (domain.RecoveryAccount, error) {
	return readRecoveryAccount(ctx, tx.tx, kind, id)
}
func (tx *Tx) LockAccount(ctx context.Context, kind, id string) error {
	table, err := accountTable(kind)
	if err != nil {
		return err
	}
	_, err = tx.tx.ExecContext(ctx, `UPDATE `+table+` SET id=id WHERE id=?`, id)
	return err
}
func (tx *Tx) ResetAccountPassword(ctx context.Context, kind, id, hash string, now time.Time) error {
	switch kind {
	case "customer":
		return tx.SetCustomerPassword(ctx, id, hash)
	case "admin":
		_, err := tx.tx.ExecContext(ctx, `UPDATE admin_users SET password_hash=?,session_version=session_version+1,updated_at=? WHERE id=?`, hash, ts(now), id)
		return err
	case "agent":
		if _, err := tx.tx.ExecContext(ctx, `UPDATE agent_users SET password_hash=?,updated_at=? WHERE id=?`, hash, ts(now), id); err != nil {
			return err
		}
		var version int
		err := tx.tx.QueryRowContext(ctx, `SELECT version FROM agent_session_versions WHERE user_id=?`, id).Scan(&version)
		if err != nil && !IsNotFound(err) {
			return err
		}
		if _, err = tx.tx.ExecContext(ctx, `DELETE FROM agent_session_versions WHERE user_id=?`, id); err != nil {
			return err
		}
		_, err = tx.tx.ExecContext(ctx, `INSERT INTO agent_session_versions (user_id,version) VALUES (?,?)`, id, version+1)
		return err
	}
	return fmt.Errorf("invalid account kind")
}
func (tx *Tx) CreatePasswordReset(ctx context.Context, r domain.PasswordReset) error {
	if _, err := tx.tx.ExecContext(ctx, `UPDATE account_password_resets SET consumed_at=? WHERE account_kind=? AND user_id=? AND consumed_at IS NULL`, ts(r.CreatedAt), r.Kind, r.UserID); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO account_password_resets (id,token_hash,account_kind,user_id,credential_hash,expires_at,created_by,reason,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, r.ID, r.TokenHash, r.Kind, r.UserID, r.CredentialHash, ts(r.ExpiresAt), r.CreatedBy, r.Reason, ts(r.CreatedAt))
	return err
}
func readReset(row scanner) (domain.PasswordReset, error) {
	var r domain.PasswordReset
	var expires, created string
	var consumed sql.NullString
	err := row.Scan(&r.ID, &r.TokenHash, &r.Kind, &r.UserID, &r.CredentialHash, &expires, &consumed, &r.CreatedBy, &r.Reason, &created)
	r.ExpiresAt = parseTS(expires)
	r.CreatedAt = parseTS(created)
	r.ConsumedAt = timePtr(consumed)
	return r, err
}

const resetCols = `id,token_hash,account_kind,user_id,credential_hash,expires_at,consumed_at,created_by,reason,created_at`

func (s *Store) PasswordReset(ctx context.Context, hash string) (domain.PasswordReset, error) {
	return readReset(s.db.QueryRowContext(ctx, `SELECT `+resetCols+` FROM account_password_resets WHERE token_hash=?`, hash))
}
func (tx *Tx) PasswordReset(ctx context.Context, hash string) (domain.PasswordReset, error) {
	return readReset(tx.tx.QueryRowContext(ctx, `SELECT `+resetCols+` FROM account_password_resets WHERE token_hash=?`, hash))
}

// PasswordResetByAccount 按账号作用域查找恢复记录（6 位验证码使用）。
func (s *Store) PasswordResetByAccount(ctx context.Context, kind, userID, tokenHash string) (domain.PasswordReset, error) {
	return readReset(s.db.QueryRowContext(ctx, `SELECT `+resetCols+` FROM account_password_resets WHERE account_kind=? AND user_id=? AND token_hash=?`, kind, userID, tokenHash))
}
func (tx *Tx) PasswordResetByAccount(ctx context.Context, kind, userID, tokenHash string) (domain.PasswordReset, error) {
	return readReset(tx.tx.QueryRowContext(ctx, `SELECT `+resetCols+` FROM account_password_resets WHERE account_kind=? AND user_id=? AND token_hash=?`, kind, userID, tokenHash))
}

// ResetAccountRefsByUsername 返回同名登录账号的定位引用。管理员与普通用户名全局
// 唯一，代理账号用户名仅代理内唯一，因此可能命中多条。
func (s *Store) ResetAccountRefsByUsername(ctx context.Context, username string) ([]domain.ResetAccountRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, 'admin' AS account_kind, status FROM admin_users WHERE username = ?
		UNION ALL
		SELECT id, username, 'customer' AS account_kind, status FROM customer_users WHERE username = ?
		UNION ALL
		SELECT id, username, 'agent' AS account_kind, status FROM agent_users WHERE username = ?`, username, username, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := []domain.ResetAccountRef{}
	for rows.Next() {
		var r domain.ResetAccountRef
		if err := rows.Scan(&r.ID, &r.Username, &r.Kind, &r.Status); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}
func (tx *Tx) ConsumePasswordReset(ctx context.Context, id string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE account_password_resets SET consumed_at=? WHERE id=?`, ts(now), id)
	return err
}

// LastPasswordResetCreated 返回该账号最近一次恢复码的签发时间（无记录返回 false）。
func (s *Store) LastPasswordResetCreated(ctx context.Context, kind, userID string) (time.Time, bool, error) {
	var created string
	err := s.db.QueryRowContext(ctx,
		`SELECT created_at FROM account_password_resets WHERE account_kind=? AND user_id=? ORDER BY created_at DESC LIMIT 1`,
		kind, userID).Scan(&created)
	if IsNotFound(err) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return parseTS(created), true, nil
}

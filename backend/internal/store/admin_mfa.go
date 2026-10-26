package store

import (
	"context"
	"time"
)

type AdminMFA struct {
	UserID, Secret, PendingAt, RecoveryHashes string
	Enabled                                   bool
	LastStep                                  int64
}

func (s *Store) AdminMFA(ctx context.Context, id string) (AdminMFA, error) {
	m := AdminMFA{UserID: id}
	err := s.db.QueryRowContext(ctx, `SELECT secret_enc,enabled,pending_at,last_step,recovery_hashes FROM admin_mfa WHERE user_id=?`, id).Scan(&m.Secret, &m.Enabled, &m.PendingAt, &m.LastStep, &m.RecoveryHashes)
	return m, err
}
func (tx *Tx) AdminMFA(ctx context.Context, id string) (AdminMFA, error) {
	if _, err := tx.tx.ExecContext(ctx, `UPDATE admin_mfa SET user_id=user_id WHERE user_id=?`, id); err != nil {
		return AdminMFA{}, err
	}
	m := AdminMFA{UserID: id}
	err := tx.tx.QueryRowContext(ctx, `SELECT secret_enc,enabled,pending_at,last_step,recovery_hashes FROM admin_mfa WHERE user_id=?`, id).Scan(&m.Secret, &m.Enabled, &m.PendingAt, &m.LastStep, &m.RecoveryHashes)
	return m, err
}
func (tx *Tx) SaveAdminMFA(ctx context.Context, m AdminMFA, revoke bool) error {
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM admin_mfa WHERE user_id=?`, m.UserID); err != nil {
		return err
	}
	if _, err := tx.tx.ExecContext(ctx, `INSERT INTO admin_mfa(user_id,secret_enc,enabled,pending_at,last_step,recovery_hashes) VALUES(?,?,?,?,?,?)`, m.UserID, m.Secret, m.Enabled, m.PendingAt, m.LastStep, m.RecoveryHashes); err != nil {
		return err
	}
	if revoke {
		_, err := tx.tx.ExecContext(ctx, `UPDATE admin_users SET session_version=session_version+1,updated_at=? WHERE id=?`, ts(time.Now().UTC()), m.UserID)
		return err
	}
	return nil
}
func (tx *Tx) LockAdmin(ctx context.Context, id string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE admin_users SET updated_at=updated_at WHERE id=?`, id)
	return err
}

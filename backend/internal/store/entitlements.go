package store

import (
	"context"
	"errors"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

var ErrLicensePolicyLocked = errors.New("product already has issued cards or licenses")

// A shared product write lock serializes policy changes with both platform and agent card issuance.
func (tx *Tx) lockProduct(ctx context.Context, productID string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE products SET updated_at = updated_at WHERE id = ?`, productID)
	return err
}

func (s *Store) LicensePolicyLocked(ctx context.Context, productID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM cards WHERE product_id = ?) + (SELECT COUNT(*) FROM licenses WHERE product_id = ?)`, productID, productID).Scan(&count)
	return count > 0, err
}

func (s *Store) SetLicensePolicy(ctx context.Context, productID, mode string, updateDays, graceDays int, now time.Time) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.lockProduct(ctx, productID); err != nil {
			return err
		}
		var count int
		if err := tx.tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM cards WHERE product_id = ?) + (SELECT COUNT(*) FROM licenses WHERE product_id = ?)`, productID, productID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return ErrLicensePolicyLocked
		}
		if _, err := tx.tx.ExecContext(ctx, `DELETE FROM product_license_policies WHERE product_id = ?`, productID); err != nil {
			return err
		}
		if _, err := tx.tx.ExecContext(ctx, `INSERT INTO product_license_policies (product_id,license_mode,update_duration_days) VALUES (?,?,?)`, productID, mode, updateDays); err != nil {
			return err
		}
		_, err := tx.tx.ExecContext(ctx, `UPDATE products SET expire_grace_days = ?, updated_at = ? WHERE id = ?`, graceDays, ts(now), productID)
		return err
	})
}

func (tx *Tx) setEntitlements(ctx context.Context, id, mode string, updateExpiry *time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM license_entitlements WHERE license_id = ?`, id); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO license_entitlements (license_id,license_mode,updates_expired_at) VALUES (?,?,?)`, id, mode, nullTime(updateExpiry))
	return err
}

func (tx *Tx) UpdateLicenseRights(ctx context.Context, lic domain.License, now time.Time) error {
	if err := tx.UpdateLicenseExpiry(ctx, lic.ID, lic.ExpiredAt, now); err != nil {
		return err
	}
	return tx.setEntitlements(ctx, lic.ID, lic.LicenseMode, lic.UpdatesExpiredAt)
}

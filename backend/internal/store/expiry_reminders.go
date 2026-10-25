package store

import (
	"context"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

// ExpiringLicense is one owner-held license whose usage or update window is
// closing; collected by the expiry reminder scan.
type ExpiringLicense struct {
	LicenseID      string
	LicenseNo      string
	ProductName    string
	UserID         string
	Email          string
	UsageExpiredAt *time.Time
	UpdatesExpired *time.Time
}

// ListExpiringLicenses returns active licenses with an owner account and an
// email whose usage or update expiry falls inside the horizon window.
func (s *Store) ListExpiringLicenses(ctx context.Context, now, horizon time.Time) ([]ExpiringLicense, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.license_no, p.name, u.id, u.email, l.expired_at,
		       COALESCE(e.updates_expired_at, l.expired_at)
		FROM licenses l
		JOIN products p ON p.id = l.product_id
		JOIN license_owners lo ON lo.license_id = l.id
		JOIN customer_users u ON u.id = lo.user_id AND u.status = 'active'
		LEFT JOIN license_entitlements e ON e.license_id = l.id
		WHERE l.status = ? AND u.email IS NOT NULL AND u.email != ''
		  AND ((l.expired_at IS NOT NULL AND l.expired_at > ? AND l.expired_at <= ?)
		    OR (COALESCE(e.updates_expired_at, l.expired_at) IS NOT NULL
		          AND COALESCE(e.updates_expired_at, l.expired_at) > ?
		          AND COALESCE(e.updates_expired_at, l.expired_at) <= ?))
		ORDER BY l.expired_at ASC
		LIMIT 2000`,
		domain.LicenseActive, ts(now), ts(horizon), ts(now), ts(horizon))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpiringLicense
	for rows.Next() {
		var item ExpiringLicense
		var usageExpired, updatesExpired *string
		if err := rows.Scan(&item.LicenseID, &item.LicenseNo, &item.ProductName,
			&item.UserID, &item.Email, &usageExpired, &updatesExpired); err != nil {
			return nil, err
		}
		if usageExpired != nil {
			t := parseTS(*usageExpired)
			item.UsageExpiredAt = &t
		}
		if updatesExpired != nil {
			t := parseTS(*updatesExpired)
			item.UpdatesExpired = &t
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// InsertExpiryReminderIfAbsent records a sent reminder and reports whether this
// call was the first one (true) or the reminder already existed (false).
func (s *Store) InsertExpiryReminderIfAbsent(ctx context.Context, reminder domain.ExpiryReminder) (bool, error) {
	ignoreClause := "OR IGNORE"
	if s.driver == "mysql" {
		ignoreClause = "IGNORE"
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT `+ignoreClause+` INTO license_expiry_reminders (
		  id, license_id, user_id, kind, expiry_value, sent_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		reminder.ID, reminder.LicenseID, reminder.UserID, reminder.Kind,
		reminder.ExpiryValue, ts(reminder.SentAt), ts(reminder.CreatedAt))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

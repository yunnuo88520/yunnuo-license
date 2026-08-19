package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func (s *Store) CreateRiskBlock(ctx context.Context, block domain.RiskBlock) error {
	upsertClause := `ON CONFLICT(kind, product_id, value_hash) DO UPDATE SET
		value_masked = excluded.value_masked,
		reason = excluded.reason,
		status = excluded.status,
		created_by = excluded.created_by,
		updated_at = excluded.updated_at`
	if s.driver == "mysql" {
		upsertClause = `ON DUPLICATE KEY UPDATE
		value_masked = VALUES(value_masked),
		reason = VALUES(reason),
		status = VALUES(status),
		created_by = VALUES(created_by),
		updated_at = VALUES(updated_at)`
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO risk_blocks (
		  id, product_id, kind, value_hash, value_masked, reason, status,
		  created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`+upsertClause,
		block.ID, block.ProductID, block.Kind, block.ValueHash, block.ValueMasked,
		block.Reason, block.Status, nullString(block.CreatedBy), ts(block.CreatedAt), ts(block.UpdatedAt),
	)
	return err
}

func (s *Store) CountRecentClientEventsByIP(ctx context.Context, productID, action, result, clientIP string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE actor_type = 'client' AND product_id = ? AND action = ? AND result = ?
		  AND client_ip = ? AND created_at >= ?`, productID, action, result, clientIP, ts(since)).Scan(&count)
	return count, err
}

func (s *Store) FindActiveRiskBlock(ctx context.Context, productID, kind, valueHash string) (domain.RiskBlock, error) {
	return scanRiskBlock(s.db.QueryRowContext(ctx, `
		SELECT `+riskBlockColumns+` FROM risk_blocks
		WHERE kind = ? AND value_hash = ? AND status = ? AND (product_id = '' OR product_id = ?)
		ORDER BY CASE WHEN product_id = ? THEN 0 ELSE 1 END
		LIMIT 1`, kind, valueHash, domain.RiskStatusActive, productID, productID))
}

func (s *Store) ListRiskBlocks(ctx context.Context, kind, status, productID string) ([]domain.RiskBlock, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+riskBlockColumns+` FROM risk_blocks
		WHERE (? = '' OR kind = ?)
		  AND (? = '' OR status = ?)
		  AND (? = '' OR product_id = ?)
		ORDER BY created_at DESC`, kind, kind, status, status, productID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var blocks []domain.RiskBlock
	for rows.Next() {
		block, err := scanRiskBlock(rows)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, rows.Err()
}

func (s *Store) DisableRiskBlock(ctx context.Context, id string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE risk_blocks SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		domain.RiskStatusDisabled, ts(now), id, domain.RiskStatusActive)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *Store) CountBindingsByHash(ctx context.Context, productID, bindHash string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT license_id) FROM license_bindings
		WHERE product_id = ? AND bind_value_hash = ?`, productID, bindHash).Scan(&count)
	return count, err
}

func (s *Store) UpsertRiskAlert(ctx context.Context, alert domain.RiskAlert) (domain.RiskAlert, error) {
	upsertClause := `ON CONFLICT(product_id, alert_type, subject_hash, open_marker) DO UPDATE SET
		occurrence_count = risk_alerts.occurrence_count + 1,
		severity = excluded.severity, detail = excluded.detail,
		license_id = excluded.license_id, binding_id = excluded.binding_id,
		subject_masked = excluded.subject_masked,
		last_seen_at = excluded.last_seen_at, updated_at = excluded.updated_at`
	if s.driver == "mysql" {
		upsertClause = `ON DUPLICATE KEY UPDATE
			occurrence_count = occurrence_count + 1,
			severity = VALUES(severity), detail = VALUES(detail),
			license_id = VALUES(license_id), binding_id = VALUES(binding_id),
			subject_masked = VALUES(subject_masked),
			last_seen_at = VALUES(last_seen_at), updated_at = VALUES(updated_at)`
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO risk_alerts (
		  id, product_id, license_id, binding_id, alert_type, severity, status,
		  subject_kind, subject_hash, subject_masked, open_marker, detail, occurrence_count,
		  first_seen_at, last_seen_at, resolved_at, resolved_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`+upsertClause,
		alert.ID, alert.ProductID, nullString(alert.LicenseID), nullString(alert.BindingID),
		alert.AlertType, alert.Severity, alert.Status, alert.SubjectKind, alert.SubjectHash,
		alert.SubjectMasked, domain.RiskAlertOpen, alert.Detail, alert.OccurrenceCount, ts(alert.FirstSeenAt),
		ts(alert.LastSeenAt), nullTime(alert.ResolvedAt), nullString(alert.ResolvedBy),
		ts(alert.CreatedAt), ts(alert.UpdatedAt),
	)
	if err != nil {
		return domain.RiskAlert{}, err
	}
	return scanRiskAlert(s.db.QueryRowContext(ctx, `
		SELECT `+riskAlertColumns+` FROM risk_alerts
		WHERE product_id = ? AND alert_type = ? AND subject_hash = ? AND open_marker = ?`,
		alert.ProductID, alert.AlertType, alert.SubjectHash, domain.RiskAlertOpen))
}

func (s *Store) ListRiskAlerts(ctx context.Context, status, severity, alertType, productID string, limit int) ([]domain.RiskAlert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+riskAlertColumns+` FROM risk_alerts
		WHERE (? = '' OR status = ?)
		  AND (? = '' OR severity = ?)
		  AND (? = '' OR alert_type = ?)
		  AND (? = '' OR product_id = ?)
		ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END,
		  last_seen_at DESC LIMIT ?`,
		status, status, severity, severity, alertType, alertType, productID, productID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var alerts []domain.RiskAlert
	for rows.Next() {
		alert, err := scanRiskAlert(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

func (s *Store) ResolveRiskAlert(ctx context.Context, id, actorID string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE risk_alerts SET status = ?, open_marker = NULL, resolved_at = ?, resolved_by = ?, updated_at = ?
		WHERE id = ? AND status = ?`, domain.RiskAlertResolved, ts(now), actorID, ts(now), id, domain.RiskAlertOpen)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *Store) RiskSummary(ctx context.Context, since time.Time) (domain.RiskSummary, error) {
	var summary domain.RiskSummary
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM risk_blocks WHERE status = ?`, domain.RiskStatusActive).Scan(&summary.ActiveBlocks); err != nil {
		return summary, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM risk_alerts WHERE status = ?`, domain.RiskAlertOpen).Scan(&summary.OpenAlerts); err != nil {
		return summary, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM risk_alerts WHERE status = ? AND severity = 'critical'`, domain.RiskAlertOpen).Scan(&summary.CriticalAlerts); err != nil {
		return summary, err
	}
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM risk_alerts WHERE last_seen_at >= ?`, ts(since)).Scan(&summary.Alerts24Hours)
	return summary, err
}

const riskBlockColumns = `id, product_id, kind, value_hash, value_masked, reason, status, created_by, created_at, updated_at`
const riskAlertColumns = `id, product_id, license_id, binding_id, alert_type, severity, status, subject_kind, subject_hash, subject_masked, detail, occurrence_count, first_seen_at, last_seen_at, resolved_at, resolved_by, created_at, updated_at`

func scanRiskBlock(row scanner) (domain.RiskBlock, error) {
	var block domain.RiskBlock
	var createdBy sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&block.ID, &block.ProductID, &block.Kind, &block.ValueHash,
		&block.ValueMasked, &block.Reason, &block.Status, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		return domain.RiskBlock{}, err
	}
	block.CreatedBy = str(createdBy)
	block.CreatedAt = parseTS(createdAt)
	block.UpdatedAt = parseTS(updatedAt)
	return block, nil
}

func scanRiskAlert(row scanner) (domain.RiskAlert, error) {
	var alert domain.RiskAlert
	var licenseID, bindingID, resolvedAt, resolvedBy sql.NullString
	var firstSeenAt, lastSeenAt, createdAt, updatedAt string
	err := row.Scan(&alert.ID, &alert.ProductID, &licenseID, &bindingID, &alert.AlertType,
		&alert.Severity, &alert.Status, &alert.SubjectKind, &alert.SubjectHash,
		&alert.SubjectMasked, &alert.Detail, &alert.OccurrenceCount, &firstSeenAt,
		&lastSeenAt, &resolvedAt, &resolvedBy, &createdAt, &updatedAt)
	if err != nil {
		return domain.RiskAlert{}, err
	}
	alert.LicenseID = str(licenseID)
	alert.BindingID = str(bindingID)
	alert.ResolvedBy = str(resolvedBy)
	alert.FirstSeenAt = parseTS(firstSeenAt)
	alert.LastSeenAt = parseTS(lastSeenAt)
	alert.ResolvedAt = timePtr(resolvedAt)
	alert.CreatedAt = parseTS(createdAt)
	alert.UpdatedAt = parseTS(updatedAt)
	return alert, nil
}

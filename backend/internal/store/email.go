package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

const emailSettingsCols = `enabled, smtp_host, smtp_port, smtp_username, smtp_password_enc, from_address, from_name, encryption, site_url, updated_at`

func readEmailSettings(row scanner) (domain.EmailSettings, error) {
	var s domain.EmailSettings
	var updatedAt string
	if err := row.Scan(&s.Enabled, &s.SMTPHost, &s.SMTPPort, &s.SMTPUsername, &s.SMTPPasswordEnc,
		&s.FromAddress, &s.FromName, &s.Encryption, &s.SiteURL, &updatedAt); err != nil {
		return domain.EmailSettings{}, err
	}
	s.UpdatedAt = parseTS(updatedAt)
	return s, nil
}

func (s *Store) GetEmailSettings(ctx context.Context) (domain.EmailSettings, error) {
	return readEmailSettings(s.db.QueryRowContext(ctx, `SELECT `+emailSettingsCols+` FROM email_settings WHERE id = 1`))
}

func (s *Store) UpdateEmailSettings(ctx context.Context, settings domain.EmailSettings) error {
	_, err := s.db.ExecContext(ctx, `UPDATE email_settings SET enabled = ?, smtp_host = ?, smtp_port = ?, smtp_username = ?, smtp_password_enc = ?, from_address = ?, from_name = ?, encryption = ?, site_url = ?, updated_at = ? WHERE id = 1`,
		settings.Enabled, settings.SMTPHost, settings.SMTPPort, settings.SMTPUsername, settings.SMTPPasswordEnc,
		settings.FromAddress, settings.FromName, settings.Encryption, settings.SiteURL, ts(settings.UpdatedAt))
	return err
}

func (s *Store) InsertEmailLog(ctx context.Context, log domain.EmailLog) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO email_logs (id, to_address, subject, kind, agent_id, status, error, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		log.ID, log.ToAddress, log.Subject, log.Kind, log.AgentID, log.Status, log.Error, ts(log.CreatedAt))
	return err
}

func (s *Store) ListEmailLogs(ctx context.Context, agentID string, limit int) ([]domain.EmailLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := `SELECT id, to_address, subject, kind, agent_id, status, error, created_at FROM email_logs`
	args := []any{}
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		query += ` WHERE agent_id = ?`
		args = append(args, agentID)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []domain.EmailLog{}
	for rows.Next() {
		var log domain.EmailLog
		var createdAt string
		if err := rows.Scan(&log.ID, &log.ToAddress, &log.Subject, &log.Kind, &log.AgentID, &log.Status, &log.Error, &createdAt); err != nil {
			return nil, err
		}
		log.CreatedAt = parseTS(createdAt)
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// AgentEmailRecipients 收集代理联系邮箱与全部启用账号邮箱（小写去重），用于邮件通知。
func (s *Store) AgentEmailRecipients(ctx context.Context, agentID string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	var agentEmail sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT email FROM agents WHERE id = ?`, agentID).Scan(&agentEmail); err != nil && !IsNotFound(err) {
		return nil, err
	}
	add(agentEmail.String)
	rows, err := s.db.QueryContext(ctx, `SELECT email FROM agent_users WHERE agent_id = ? AND status = 'active'`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var email sql.NullString
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		add(email.String)
	}
	return out, rows.Err()
}

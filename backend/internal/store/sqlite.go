package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

type Store struct {
	db     *sql.DB
	driver string
}

type Tx struct {
	tx *sql.Tx
}

func Open(ctx context.Context, driver, dsn string) (*Store, error) {
	if driver != "sqlite" && driver != "mysql" {
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if driver == "mysql" {
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(5 * time.Minute)
	}
	return &Store{db: db, driver: driver}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CountAdminUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&count)
	return count, err
}

func (s *Store) CreateAdminUser(ctx context.Context, user domain.AdminUser) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_users (
		  id, username, password_hash, display_name, role, status, session_version,
		  last_login_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash, nullString(user.DisplayName),
		user.Role, user.Status, user.SessionVersion, nullTime(user.LastLoginAt),
		ts(user.CreatedAt), ts(user.UpdatedAt),
	)
	return err
}

func (s *Store) GetAdminUserByUsername(ctx context.Context, username string) (domain.AdminUser, error) {
	return scanAdminUser(s.db.QueryRowContext(ctx, `SELECT `+adminUserColumns+` FROM admin_users WHERE username = ?`, username))
}

func (s *Store) GetAdminUserByID(ctx context.Context, userID string) (domain.AdminUser, error) {
	return scanAdminUser(s.db.QueryRowContext(ctx, `SELECT `+adminUserColumns+` FROM admin_users WHERE id = ?`, userID))
}

func (s *Store) ListAdminUsers(ctx context.Context) ([]domain.AdminUser, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+adminUserColumns+` FROM admin_users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []domain.AdminUser
	for rows.Next() {
		user, err := scanAdminUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UpdateAdminUserLastLogin(ctx context.Context, userID string, lastLoginAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE admin_users SET last_login_at = ?, updated_at = ? WHERE id = ?`, ts(lastLoginAt), ts(lastLoginAt), userID)
	return err
}

func (s *Store) UpdateAdminUserPassword(ctx context.Context, userID, passwordHash string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_users
		SET password_hash = ?, session_version = session_version + 1, updated_at = ?
		WHERE id = ?`, passwordHash, ts(now), userID)
	return err
}

func (s *Store) Migrate(ctx context.Context, dir string) error {
	if s.driver == "mysql" {
		dir = filepath.Join(dir, "mysql")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		sqlText, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, string(sqlText)); err != nil {
			return fmt.Errorf("run migration %s: %w", filepath.Base(file), err)
		}
	}
	return nil
}

func (s *Store) WithTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Tx{tx: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateProduct(ctx context.Context, p domain.Product) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		if _, err := tx.tx.ExecContext(ctx, `
		INSERT INTO products (
		  id, name, code, app_key, public_key_pem, private_key_encrypted,
		  bind_mode, max_bind_count, bind_conflict_strategy, offline_mode,
		  offline_grace_days, expire_grace_days, unbind_limit, unbind_cooldown_hours,
		  status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Name, p.Code, p.AppKey, p.PublicKeyPEM, p.PrivateKeyEncrypted,
			p.BindMode, p.MaxBindCount, p.BindConflictStrategy, p.OfflineMode,
			p.OfflineGraceDays, p.ExpireGraceDays, p.UnbindLimit, p.UnbindCooldownHours,
			p.Status, ts(p.CreatedAt), ts(p.UpdatedAt),
		); err != nil {
			return err
		}
		_, err := tx.tx.ExecContext(ctx, `
			INSERT INTO product_keys (product_id, key_version, public_key_pem, created_by, created_at)
			VALUES (?, 1, ?, ?, ?)`, p.ID, p.PublicKeyPEM, "product.create", ts(p.CreatedAt))
		return err
	})
}

func (s *Store) GetProductByAppKey(ctx context.Context, appKey string) (domain.Product, error) {
	return scanProduct(s.db.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE app_key = ?`, appKey))
}

func (s *Store) GetProduct(ctx context.Context, id string) (domain.Product, error) {
	return scanProduct(s.db.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id = ?`, id))
}

func (s *Store) ListProducts(ctx context.Context) ([]domain.Product, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+productColumns+` FROM products ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []domain.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (s *Store) ListProductKeys(ctx context.Context, productID string) ([]domain.ProductKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT product_id, key_version, public_key_pem, created_by, created_at
		FROM product_keys WHERE product_id = ? ORDER BY key_version DESC`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []domain.ProductKey
	for rows.Next() {
		var key domain.ProductKey
		var createdBy sql.NullString
		var createdAt string
		if err := rows.Scan(&key.ProductID, &key.KeyVersion, &key.PublicKeyPEM, &createdBy, &createdAt); err != nil {
			return nil, err
		}
		key.CreatedBy = str(createdBy)
		key.CreatedAt = parseTS(createdAt)
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) RotateProductKey(ctx context.Context, productID, publicKeyPEM, privateKeyEncrypted, actorID string, now time.Time) (int, error) {
	var version int
	err := s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(key_version), 0) + 1 FROM product_keys WHERE product_id = ?`, productID).Scan(&version); err != nil {
			return err
		}
		result, err := tx.tx.ExecContext(ctx, `
			UPDATE products SET public_key_pem = ?, private_key_encrypted = ?, updated_at = ? WHERE id = ?`,
			publicKeyPEM, privateKeyEncrypted, ts(now), productID)
		if err != nil {
			return err
		}
		if err := requireAffected(result); err != nil {
			return err
		}
		_, err = tx.tx.ExecContext(ctx, `
			INSERT INTO product_keys (product_id, key_version, public_key_pem, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)`, productID, version, publicKeyPEM, nullString(actorID), ts(now))
		return err
	})
	return version, err
}

func (s *Store) CreateOfflineLicense(ctx context.Context, license domain.OfflineLicense) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO offline_licenses (
		  id, license_no, product_id, label, machine_code_hash, machine_code_encrypted,
		  machine_code_masked, signed_token_encrypted, token_version, status, issued_at,
		  expired_at, revoked_at, revoked_reason, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		license.ID, license.LicenseNo, license.ProductID, nullString(license.Label),
		license.MachineCodeHash, license.MachineCodeEncrypted, license.MachineCodeMasked,
		license.SignedTokenEncrypted, license.TokenVersion, license.Status, ts(license.IssuedAt),
		nullTime(license.ExpiredAt), nullTime(license.RevokedAt), nullString(license.RevokedReason),
		nullString(license.CreatedBy), ts(license.CreatedAt), ts(license.UpdatedAt),
	)
	return err
}

func (s *Store) GetOfflineLicense(ctx context.Context, id string) (domain.OfflineLicense, error) {
	return scanOfflineLicense(s.db.QueryRowContext(ctx,
		`SELECT `+offlineLicenseColumns+` FROM offline_licenses WHERE id = ?`, id))
}

func (s *Store) ListOfflineLicenses(ctx context.Context) ([]domain.OfflineLicense, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+offlineLicenseColumns+` FROM offline_licenses ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var licenses []domain.OfflineLicense
	for rows.Next() {
		license, err := scanOfflineLicense(rows)
		if err != nil {
			return nil, err
		}
		licenses = append(licenses, license)
	}
	return licenses, rows.Err()
}

func (s *Store) RevokeOfflineLicense(ctx context.Context, id, reason string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE offline_licenses
		SET status = ?, revoked_at = ?, revoked_reason = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		domain.LicenseRevoked, ts(now), nullString(reason), ts(now), id, domain.LicenseActive)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *Store) UpdateProductStatus(ctx context.Context, productID, status string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE products SET status = ?, updated_at = ? WHERE id = ?`, status, ts(now), productID)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *Store) CreateCardBatch(ctx context.Context, batch domain.CardBatch, cards []domain.Card) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.CreateCardBatch(ctx, batch); err != nil {
			return err
		}
		for _, card := range cards {
			if err := tx.CreateCard(ctx, card); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ListCardBatches(ctx context.Context) ([]domain.CardBatch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+batchColumns+` FROM card_batches ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var batches []domain.CardBatch
	for rows.Next() {
		batch, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}
	return batches, rows.Err()
}

func (s *Store) GetCardBatch(ctx context.Context, batchID string) (domain.CardBatch, error) {
	return scanBatch(s.db.QueryRowContext(ctx, `SELECT `+batchColumns+` FROM card_batches WHERE id = ?`, batchID))
}

func (s *Store) IncrementCardBatchExportCount(ctx context.Context, batchID string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE card_batches SET export_count = export_count + 1, updated_at = ? WHERE id = ?`, ts(now), batchID)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *Store) ListCardBatchesByAgent(ctx context.Context, agentID string) ([]domain.CardBatch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+batchColumns+` FROM card_batches WHERE agent_id = ? ORDER BY created_at DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var batches []domain.CardBatch
	for rows.Next() {
		batch, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}
	return batches, rows.Err()
}

func (s *Store) GetCardBatchForAgent(ctx context.Context, batchID, agentID string) (domain.CardBatch, error) {
	return scanBatch(s.db.QueryRowContext(ctx, `SELECT `+batchColumns+` FROM card_batches WHERE id = ? AND agent_id = ?`, batchID, agentID))
}

func (s *Store) ListCardsByBatch(ctx context.Context, batchID string) ([]domain.Card, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cardColumns+` FROM cards WHERE batch_id = ? ORDER BY created_at`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []domain.Card
	for rows.Next() {
		card, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func (s *Store) ListCardsByBatchForAgent(ctx context.Context, batchID, agentID string) ([]domain.Card, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cardColumns+` FROM cards WHERE batch_id = ? AND agent_id = ? ORDER BY created_at`, batchID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []domain.Card
	for rows.Next() {
		card, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func (s *Store) GetCardByHash(ctx context.Context, codeHash string) (domain.Card, error) {
	return scanCard(s.db.QueryRowContext(ctx, `SELECT `+cardColumns+` FROM cards WHERE code_hash = ?`, codeHash))
}

func (s *Store) GetCard(ctx context.Context, cardID string) (domain.Card, error) {
	return scanCard(s.db.QueryRowContext(ctx, `SELECT `+cardColumns+` FROM cards WHERE id = ?`, cardID))
}

func (s *Store) UpdateCardVoided(ctx context.Context, cardID, reason string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE cards
		SET status = ?, voided_at = ?, void_reason = ?, updated_at = ?
		WHERE id = ? AND status = ?`, domain.CardVoided, ts(now), reason, ts(now), cardID, domain.CardUnused)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *Store) GetLicenseByNo(ctx context.Context, licenseNo string) (domain.License, error) {
	return scanLicense(s.db.QueryRowContext(ctx, `SELECT `+licenseColumns+` FROM licenses WHERE license_no = ?`, licenseNo))
}

func (s *Store) GetLicenseByID(ctx context.Context, licenseID string) (domain.License, error) {
	return scanLicense(s.db.QueryRowContext(ctx, `SELECT `+licenseColumns+` FROM licenses WHERE id = ?`, licenseID))
}

func (s *Store) ListLicenses(ctx context.Context) ([]domain.License, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+licenseColumns+` FROM licenses ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var licenses []domain.License
	for rows.Next() {
		lic, err := scanLicense(rows)
		if err != nil {
			return nil, err
		}
		licenses = append(licenses, lic)
	}
	return licenses, rows.Err()
}

func (s *Store) ListLicensesPage(ctx context.Context, status, productID, agentID, query string, limit, offset int, now time.Time) ([]domain.License, int, error) {
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 8)
	if status != "" {
		switch status {
		case domain.LicenseActive:
			clauses = append(clauses, `status = ? AND (expired_at IS NULL OR expired_at > ?)`)
			args = append(args, domain.LicenseActive, ts(now))
		case domain.LicenseExpired:
			clauses = append(clauses, `(status = ? OR (status = ? AND expired_at IS NOT NULL AND expired_at <= ?))`)
			args = append(args, domain.LicenseExpired, domain.LicenseActive, ts(now))
		default:
			clauses = append(clauses, `status = ?`)
			args = append(args, status)
		}
	}
	if productID != "" {
		clauses = append(clauses, `product_id = ?`)
		args = append(args, productID)
	}
	if agentID != "" {
		clauses = append(clauses, `agent_id = ?`)
		args = append(args, agentID)
	}
	if query != "" {
		clauses = append(clauses, `license_no LIKE ?`)
		args = append(args, "%"+query+"%")
	}
	whereSQL := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM licenses WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT `+licenseColumns+` FROM licenses WHERE `+whereSQL+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	licenses := make([]domain.License, 0, min(limit, total))
	for rows.Next() {
		license, err := scanLicense(rows)
		if err != nil {
			return nil, 0, err
		}
		licenses = append(licenses, license)
	}
	return licenses, total, rows.Err()
}

func (s *Store) ListLicensesByAgent(ctx context.Context, agentID string) ([]domain.License, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+licenseColumns+` FROM licenses WHERE agent_id = ? ORDER BY created_at DESC LIMIT 200`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var licenses []domain.License
	for rows.Next() {
		license, err := scanLicense(rows)
		if err != nil {
			return nil, err
		}
		licenses = append(licenses, license)
	}
	return licenses, rows.Err()
}

func (s *Store) GetBindings(ctx context.Context, licenseID string) ([]domain.Binding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+bindingColumns+` FROM license_bindings WHERE license_id = ? ORDER BY created_at`, licenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bindings []domain.Binding
	for rows.Next() {
		binding, err := scanBinding(rows)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (s *Store) InsertAudit(ctx context.Context, log domain.AuditLog) error {
	_, err := s.db.ExecContext(ctx, auditInsertSQL,
		log.ID, log.ActorType, nullString(log.ActorID), nullString(log.AgentID),
		nullString(log.ProductID), nullString(log.LicenseID), nullString(log.CardID),
		nullString(log.BindingID), log.Action, nullString(log.ClientIP),
		nullString(log.UserAgent), nullString(log.RequestID), log.Result,
		nullString(log.ErrorCode), nullString(log.ExtraJSON), ts(log.CreatedAt),
	)
	return err
}

func (s *Store) ListAuditLogs(ctx context.Context, actorType, result, action string, limit int) ([]domain.AuditLog, error) {
	actionPredicate := `(? = '' OR action LIKE '%' || ? || '%')`
	if s.driver == "mysql" {
		actionPredicate = `(? = '' OR action LIKE CONCAT('%', ?, '%'))`
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+auditColumns+`
		FROM audit_logs
		WHERE (? = '' OR actor_type = ?)
		  AND (? = '' OR result = ?)
		  AND `+actionPredicate+`
		ORDER BY created_at DESC
		LIMIT ?`, actorType, actorType, result, result, action, action, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []domain.AuditLog
	for rows.Next() {
		log, err := scanAuditLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (s *Store) CreateAgent(ctx context.Context, agent domain.Agent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agents (
		  id, agent_no, parent_agent_id, name, contact_name, phone, email, level,
		  status, settlement_mode, default_discount_rate, credit_limit, remark,
		  created_by, created_at, updated_at, disabled_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		agent.ID, agent.AgentNo, nullString(agent.ParentAgentID), agent.Name,
		nullString(agent.ContactName), nullString(agent.Phone), nullString(agent.Email),
		agent.Level, agent.Status, agent.SettlementMode, agent.DefaultDiscountRate,
		agent.CreditLimit, nullString(agent.Remark), nullString(agent.CreatedBy),
		ts(agent.CreatedAt), ts(agent.UpdatedAt), nullTime(agent.DisabledAt),
	)
	return err
}

func (s *Store) CreateAgentWithLoginCode(ctx context.Context, agent domain.Agent) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.tx.ExecContext(ctx, `
			INSERT INTO agents (
			  id, agent_no, parent_agent_id, name, contact_name, phone, email, level,
			  status, settlement_mode, default_discount_rate, credit_limit, remark,
			  created_by, created_at, updated_at, disabled_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			agent.ID, agent.AgentNo, nullString(agent.ParentAgentID), agent.Name,
			nullString(agent.ContactName), nullString(agent.Phone), nullString(agent.Email),
			agent.Level, agent.Status, agent.SettlementMode, agent.DefaultDiscountRate,
			agent.CreditLimit, nullString(agent.Remark), nullString(agent.CreatedBy),
			ts(agent.CreatedAt), ts(agent.UpdatedAt), nullTime(agent.DisabledAt),
		)
		if err != nil {
			return err
		}
		_, err = tx.tx.ExecContext(ctx, `INSERT INTO agent_login_codes (agent_id, login_code, created_at) VALUES (?, ?, ?)`,
			agent.ID, agent.LoginCode, ts(agent.CreatedAt))
		return err
	})
}

func (s *Store) CreateAgentLoginCode(ctx context.Context, agentID, loginCode string, createdAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_login_codes (agent_id, login_code, created_at) VALUES (?, ?, ?)`, agentID, loginCode, ts(createdAt))
	return err
}

func (s *Store) GetAgentLoginCode(ctx context.Context, agentID string) (string, error) {
	var loginCode string
	err := s.db.QueryRowContext(ctx, `SELECT login_code FROM agent_login_codes WHERE agent_id = ?`, agentID).Scan(&loginCode)
	return loginCode, err
}

func (s *Store) GetAgentByLoginCode(ctx context.Context, loginCode string) (domain.Agent, error) {
	agent, err := scanAgent(s.db.QueryRowContext(ctx, `
		SELECT `+agentColumns+`
		FROM agents
		WHERE id = (SELECT agent_id FROM agent_login_codes WHERE login_code = ?)`, loginCode))
	if err != nil {
		return domain.Agent{}, err
	}
	agent.LoginCode = loginCode
	return agent, nil
}

func (s *Store) ListAgents(ctx context.Context) ([]domain.Agent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentColumns+` FROM agents ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []domain.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range agents {
		loginCode, err := s.GetAgentLoginCode(ctx, agents[i].ID)
		if err != nil && !IsNotFound(err) {
			return nil, err
		}
		agents[i].LoginCode = loginCode
	}
	return agents, nil
}

func (s *Store) GetAgent(ctx context.Context, agentID string) (domain.Agent, error) {
	return scanAgent(s.db.QueryRowContext(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = ?`, agentID))
}

func (s *Store) GetAgentByNo(ctx context.Context, agentNo string) (domain.Agent, error) {
	return scanAgent(s.db.QueryRowContext(ctx, `SELECT `+agentColumns+` FROM agents WHERE agent_no = ?`, agentNo))
}

func (s *Store) UpdateAgentStatus(ctx context.Context, agentID, status string, disabledAt *time.Time, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE agents SET status = ?, disabled_at = ?, updated_at = ? WHERE id = ?`,
		status, nullTime(disabledAt), ts(now), agentID)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *Store) CreateAgentUser(ctx context.Context, user domain.AgentUser) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_users (
		  id, agent_id, username, password_hash, display_name, phone, email,
		  role, status, last_login_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.AgentID, user.Username, user.PasswordHash, nullString(user.DisplayName),
		nullString(user.Phone), nullString(user.Email), user.Role, user.Status,
		nullTime(user.LastLoginAt), ts(user.CreatedAt), ts(user.UpdatedAt),
	)
	return err
}

func (s *Store) GetAgentUserByUsername(ctx context.Context, agentID, username string) (domain.AgentUser, error) {
	return scanAgentUser(s.db.QueryRowContext(ctx, `SELECT `+agentUserColumns+` FROM agent_users WHERE agent_id = ? AND username = ?`, agentID, username))
}

func (s *Store) GetAgentUserByID(ctx context.Context, userID string) (domain.AgentUser, error) {
	return scanAgentUser(s.db.QueryRowContext(ctx, `SELECT `+agentUserColumns+` FROM agent_users WHERE id = ?`, userID))
}

func (s *Store) ListAgentUsers(ctx context.Context, agentID string) ([]domain.AgentUser, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentUserColumns+` FROM agent_users WHERE agent_id = ? ORDER BY created_at DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []domain.AgentUser
	for rows.Next() {
		user, err := scanAgentUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UpdateAgentUserLastLogin(ctx context.Context, userID string, lastLoginAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agent_users SET last_login_at = ?, updated_at = ? WHERE id = ?`, ts(lastLoginAt), ts(lastLoginAt), userID)
	return err
}

func (s *Store) UpdateAgentUserStatus(ctx context.Context, agentID, userID, status string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE agent_users SET status = ?, updated_at = ? WHERE id = ? AND agent_id = ?`,
		status, ts(now), userID, agentID)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *Store) UpsertAgentProductPolicy(ctx context.Context, policy domain.AgentProductPolicy) error {
	durations, err := json.Marshal(policy.AllowedDurationDays)
	if err != nil {
		return err
	}
	upsertClause := `ON CONFLICT(agent_id, product_id) DO UPDATE SET
		  can_generate = excluded.can_generate,
		  can_export_plain_code = excluded.can_export_plain_code,
		  allowed_duration_days = excluded.allowed_duration_days,
		  allow_permanent = excluded.allow_permanent,
		  max_batch_quantity = excluded.max_batch_quantity,
		  discount_rate = excluded.discount_rate,
		  settlement_price = excluded.settlement_price,
		  status = excluded.status,
		  updated_at = excluded.updated_at`
	if s.driver == "mysql" {
		upsertClause = `ON DUPLICATE KEY UPDATE
		  can_generate = VALUES(can_generate),
		  can_export_plain_code = VALUES(can_export_plain_code),
		  allowed_duration_days = VALUES(allowed_duration_days),
		  allow_permanent = VALUES(allow_permanent),
		  max_batch_quantity = VALUES(max_batch_quantity),
		  discount_rate = VALUES(discount_rate),
		  settlement_price = VALUES(settlement_price),
		  status = VALUES(status),
		  updated_at = VALUES(updated_at)`
	}
	_, err = s.db.ExecContext(ctx, `
			INSERT INTO agent_product_policies (
		  id, agent_id, product_id, can_generate, can_export_plain_code,
		  allowed_duration_days, allow_permanent, max_batch_quantity,
		  discount_rate, settlement_price, status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`+upsertClause,
		policy.ID, policy.AgentID, policy.ProductID, boolInt(policy.CanGenerate),
		boolInt(policy.CanExportPlainCode), string(durations), boolInt(policy.AllowPermanent),
		policy.MaxBatchQuantity, policy.DiscountRate, policy.SettlementPrice, policy.Status,
		ts(policy.CreatedAt), ts(policy.UpdatedAt),
	)
	return err
}

func (s *Store) ListAgentProductPolicies(ctx context.Context, agentID string) ([]domain.AgentProductPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentPolicyColumns+` FROM agent_product_policies WHERE agent_id = ? ORDER BY created_at DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []domain.AgentProductPolicy
	for rows.Next() {
		policy, err := scanAgentProductPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func (s *Store) GetAgentProductPolicy(ctx context.Context, agentID, productID string) (domain.AgentProductPolicy, error) {
	return scanAgentProductPolicy(s.db.QueryRowContext(ctx, `SELECT `+agentPolicyColumns+` FROM agent_product_policies WHERE agent_id = ? AND product_id = ?`, agentID, productID))
}

func (s *Store) ListAgentQuotaLedgers(ctx context.Context, agentID string) ([]domain.AgentQuotaLedger, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentQuotaLedgerColumns+` FROM agent_quota_ledgers WHERE agent_id = ? ORDER BY created_at DESC LIMIT 200`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ledgers []domain.AgentQuotaLedger
	for rows.Next() {
		ledger, err := scanAgentQuotaLedger(rows)
		if err != nil {
			return nil, err
		}
		ledgers = append(ledgers, ledger)
	}
	return ledgers, rows.Err()
}

func (s *Store) ListAgentQuotaSummaries(ctx context.Context, agentID string) ([]domain.AgentQuotaSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT agent_id, product_id, duration_days, is_permanent, COALESCE(SUM(change_quantity), 0)
		FROM agent_quota_ledgers
		WHERE agent_id = ?
		GROUP BY agent_id, product_id, duration_days, is_permanent
		ORDER BY product_id, duration_days`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summaries []domain.AgentQuotaSummary
	for rows.Next() {
		var summary domain.AgentQuotaSummary
		var isPermanent int
		if err := rows.Scan(&summary.AgentID, &summary.ProductID, &summary.DurationDays, &isPermanent, &summary.Balance); err != nil {
			return nil, err
		}
		summary.IsPermanent = isPermanent == 1
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

func (tx *Tx) CreateCardBatch(ctx context.Context, batch domain.CardBatch) error {
	_, err := tx.tx.ExecContext(ctx, `
		INSERT INTO card_batches (
		  id, product_id, agent_id, name, quantity, duration_days, is_permanent,
		  source, status, export_count, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		batch.ID, batch.ProductID, nullString(batch.AgentID), batch.Name, batch.Quantity,
		batch.DurationDays, boolInt(batch.IsPermanent), batch.Source, batch.Status,
		batch.ExportCount, nullString(batch.CreatedBy), ts(batch.CreatedAt), ts(batch.UpdatedAt),
	)
	return err
}

func (tx *Tx) CreateCard(ctx context.Context, card domain.Card) error {
	_, err := tx.tx.ExecContext(ctx, `
		INSERT INTO cards (
		  id, product_id, batch_id, agent_id, code_hash, code_encrypted, code_prefix,
		  duration_days, is_permanent, status, order_no, activated_license_id,
		  activated_at, consumed_at, voided_at, void_reason, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		card.ID, card.ProductID, card.BatchID, nullString(card.AgentID), card.CodeHash,
		card.CodeEncrypted, card.CodePrefix, card.DurationDays, boolInt(card.IsPermanent),
		card.Status, nullString(card.OrderNo), nullString(card.ActivatedLicenseID),
		nullTime(card.ActivatedAt), nullTime(card.ConsumedAt), nullTime(card.VoidedAt),
		nullString(card.VoidReason), nullString(card.CreatedBy), ts(card.CreatedAt), ts(card.UpdatedAt),
	)
	return err
}

func (tx *Tx) GetCardByHash(ctx context.Context, codeHash string) (domain.Card, error) {
	return scanCard(tx.tx.QueryRowContext(ctx, `SELECT `+cardColumns+` FROM cards WHERE code_hash = ?`, codeHash))
}

func (tx *Tx) UpdateCardActivated(ctx context.Context, cardID, licenseID string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `
		UPDATE cards SET status = ?, activated_license_id = ?, activated_at = ?, updated_at = ?
		WHERE id = ?`,
		domain.CardActivated, licenseID, ts(now), ts(now), cardID,
	)
	return err
}

func (tx *Tx) UpdateCardConsumed(ctx context.Context, cardID string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `
		UPDATE cards SET status = ?, consumed_at = ?, updated_at = ? WHERE id = ?`,
		domain.CardConsumedForRenewal, ts(now), ts(now), cardID,
	)
	return err
}

func (tx *Tx) CreateLicense(ctx context.Context, lic domain.License) error {
	_, err := tx.tx.ExecContext(ctx, `
		INSERT INTO licenses (
		  id, license_no, product_id, card_id, agent_id, status, issued_at, activated_at,
		  expired_at, last_verify_at, last_heartbeat_at, offline_token_version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lic.ID, lic.LicenseNo, lic.ProductID, lic.CardID, nullString(lic.AgentID),
		lic.Status, ts(lic.IssuedAt), ts(lic.ActivatedAt), nullTime(lic.ExpiredAt),
		nullTime(lic.LastVerifyAt), nullTime(lic.LastHeartbeatAt), lic.OfflineTokenVersion,
		ts(lic.CreatedAt), ts(lic.UpdatedAt),
	)
	return err
}

func (tx *Tx) GetLicenseByID(ctx context.Context, id string) (domain.License, error) {
	return scanLicense(tx.tx.QueryRowContext(ctx, `SELECT `+licenseColumns+` FROM licenses WHERE id = ?`, id))
}

func (tx *Tx) GetLicenseByNo(ctx context.Context, licenseNo string) (domain.License, error) {
	return scanLicense(tx.tx.QueryRowContext(ctx, `SELECT `+licenseColumns+` FROM licenses WHERE license_no = ?`, licenseNo))
}

func (tx *Tx) GetAgent(ctx context.Context, agentID string) (domain.Agent, error) {
	return scanAgent(tx.tx.QueryRowContext(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = ?`, agentID))
}

func (tx *Tx) GetAgentProductPolicy(ctx context.Context, agentID, productID string) (domain.AgentProductPolicy, error) {
	return scanAgentProductPolicy(tx.tx.QueryRowContext(ctx, `SELECT `+agentPolicyColumns+` FROM agent_product_policies WHERE agent_id = ? AND product_id = ?`, agentID, productID))
}

func (tx *Tx) AgentQuotaBalance(ctx context.Context, agentID, productID string, durationDays int, isPermanent bool) (int, error) {
	var balance int
	err := tx.tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(change_quantity), 0)
		FROM agent_quota_ledgers
		WHERE agent_id = ? AND product_id = ? AND duration_days = ? AND is_permanent = ?`,
		agentID, productID, durationDays, boolInt(isPermanent),
	).Scan(&balance)
	return balance, err
}

func (tx *Tx) InsertAgentQuotaLedger(ctx context.Context, ledger domain.AgentQuotaLedger) error {
	_, err := tx.tx.ExecContext(ctx, `
		INSERT INTO agent_quota_ledgers (
		  id, agent_id, product_id, duration_days, is_permanent, change_type,
		  change_quantity, balance_after, related_batch_id, related_card_id,
		  operator_type, operator_id, remark, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ledger.ID, ledger.AgentID, ledger.ProductID, ledger.DurationDays,
		boolInt(ledger.IsPermanent), ledger.ChangeType, ledger.ChangeQuantity,
		ledger.BalanceAfter, nullString(ledger.RelatedBatchID), nullString(ledger.RelatedCardID),
		ledger.OperatorType, nullString(ledger.OperatorID), nullString(ledger.Remark), ts(ledger.CreatedAt),
	)
	return err
}

func (tx *Tx) ActiveBindingByHash(ctx context.Context, licenseID, bindMode, bindHash string) (domain.Binding, error) {
	return scanBinding(tx.tx.QueryRowContext(ctx, `SELECT `+bindingColumns+` FROM license_bindings WHERE license_id = ? AND bind_mode = ? AND bind_value_hash = ? AND status = ?`,
		licenseID, bindMode, bindHash, domain.BindingActive))
}

func (tx *Tx) GetBindingByID(ctx context.Context, bindingID string) (domain.Binding, error) {
	return scanBinding(tx.tx.QueryRowContext(ctx, `SELECT `+bindingColumns+` FROM license_bindings WHERE id = ?`, bindingID))
}

func (tx *Tx) CountActiveBindings(ctx context.Context, licenseID string) (int, error) {
	var count int
	err := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM license_bindings WHERE license_id = ? AND status = ?`, licenseID, domain.BindingActive).Scan(&count)
	return count, err
}

func (tx *Tx) OldestActiveBinding(ctx context.Context, licenseID string) (domain.Binding, error) {
	return scanBinding(tx.tx.QueryRowContext(ctx, `SELECT `+bindingColumns+` FROM license_bindings WHERE license_id = ? AND status = ? ORDER BY COALESCE(last_heartbeat_at, activated_at) ASC LIMIT 1`,
		licenseID, domain.BindingActive))
}

func (tx *Tx) MarkBindingKicked(ctx context.Context, bindingID string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE license_bindings SET status = ?, kicked_at = ?, updated_at = ? WHERE id = ?`,
		domain.BindingKicked, ts(now), ts(now), bindingID)
	return err
}

func (tx *Tx) MarkBindingUnbound(ctx context.Context, bindingID string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE license_bindings SET status = ?, unbound_at = ?, updated_at = ? WHERE id = ?`,
		domain.BindingUnbound, ts(now), ts(now), bindingID)
	return err
}

func (tx *Tx) RevokeLicense(ctx context.Context, licenseID, reason string, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `
		UPDATE licenses SET status = ?, revoked_at = ?, revoked_reason = ?, updated_at = ?
		WHERE id = ?`,
		domain.LicenseRevoked, ts(now), nullString(reason), ts(now), licenseID,
	); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `
		UPDATE license_bindings SET status = ?, revoked_at = ?, updated_at = ?
		WHERE license_id = ? AND status = ?`,
		domain.BindingRevoked, ts(now), ts(now), licenseID, domain.BindingActive,
	)
	return err
}

func (tx *Tx) CreateBinding(ctx context.Context, binding domain.Binding) error {
	_, err := tx.tx.ExecContext(ctx, `
		INSERT INTO license_bindings (
		  id, license_id, product_id, bind_mode, bind_value_hash, bind_value_encrypted,
		  display_name, status, first_seen_ip, last_seen_ip, user_agent,
		  last_heartbeat_at, activated_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		binding.ID, binding.LicenseID, binding.ProductID, binding.BindMode, binding.BindValueHash,
		binding.BindValueEncrypted, nullString(binding.DisplayName), binding.Status,
		nullString(binding.FirstSeenIP), nullString(binding.LastSeenIP), nullString(binding.UserAgent),
		nullTime(binding.LastHeartbeatAt), ts(binding.ActivatedAt), ts(binding.CreatedAt), ts(binding.UpdatedAt),
	)
	return err
}

func (tx *Tx) UpdateLicenseVerify(ctx context.Context, licenseID string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE licenses SET last_verify_at = ?, updated_at = ? WHERE id = ?`, ts(now), ts(now), licenseID)
	return err
}

func (tx *Tx) UpdateHeartbeat(ctx context.Context, licenseID, bindingID string, now time.Time, ip string) error {
	if _, err := tx.tx.ExecContext(ctx, `UPDATE licenses SET last_heartbeat_at = ?, updated_at = ? WHERE id = ?`, ts(now), ts(now), licenseID); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `UPDATE license_bindings SET last_heartbeat_at = ?, last_seen_ip = ?, updated_at = ? WHERE id = ?`,
		ts(now), nullString(ip), ts(now), bindingID)
	return err
}

func (tx *Tx) UpdateLicenseExpiry(ctx context.Context, licenseID string, expiredAt *time.Time, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE licenses SET status = ?, expired_at = ?, updated_at = ? WHERE id = ?`,
		domain.LicenseActive, nullTime(expiredAt), ts(now), licenseID)
	return err
}

func (tx *Tx) InsertAudit(ctx context.Context, log domain.AuditLog) error {
	_, err := tx.tx.ExecContext(ctx, auditInsertSQL,
		log.ID, log.ActorType, nullString(log.ActorID), nullString(log.AgentID),
		nullString(log.ProductID), nullString(log.LicenseID), nullString(log.CardID),
		nullString(log.BindingID), log.Action, nullString(log.ClientIP),
		nullString(log.UserAgent), nullString(log.RequestID), log.Result,
		nullString(log.ErrorCode), nullString(log.ExtraJSON), ts(log.CreatedAt),
	)
	return err
}

const productColumns = `id, name, code, app_key, public_key_pem, private_key_encrypted, COALESCE((SELECT MAX(key_version) FROM product_keys WHERE product_id = products.id), 1), bind_mode, max_bind_count, bind_conflict_strategy, offline_mode, offline_grace_days, expire_grace_days, unbind_limit, unbind_cooldown_hours, status, created_at, updated_at`
const batchColumns = `id, product_id, agent_id, name, quantity, duration_days, is_permanent, source, status, export_count, created_by, created_at, updated_at`
const cardColumns = `id, product_id, batch_id, agent_id, code_hash, code_encrypted, code_prefix, duration_days, is_permanent, status, order_no, activated_license_id, activated_at, consumed_at, voided_at, void_reason, created_by, created_at, updated_at`
const licenseColumns = `id, license_no, product_id, card_id, agent_id, status, issued_at, activated_at, expired_at, last_verify_at, last_heartbeat_at, offline_token_version, created_at, updated_at`
const offlineLicenseColumns = `id, license_no, product_id, label, machine_code_hash, machine_code_encrypted, machine_code_masked, signed_token_encrypted, token_version, status, issued_at, expired_at, revoked_at, revoked_reason, created_by, created_at, updated_at`
const bindingColumns = `id, license_id, product_id, bind_mode, bind_value_hash, bind_value_encrypted, display_name, status, first_seen_ip, last_seen_ip, user_agent, last_heartbeat_at, activated_at, created_at, updated_at`
const agentColumns = `id, agent_no, parent_agent_id, name, contact_name, phone, email, level, status, settlement_mode, default_discount_rate, credit_limit, remark, created_by, created_at, updated_at, disabled_at`
const agentUserColumns = `id, agent_id, username, password_hash, display_name, phone, email, role, status, last_login_at, created_at, updated_at`
const adminUserColumns = `id, username, password_hash, display_name, role, status, session_version, last_login_at, created_at, updated_at`
const agentPolicyColumns = `id, agent_id, product_id, can_generate, can_export_plain_code, allowed_duration_days, allow_permanent, max_batch_quantity, discount_rate, settlement_price, status, created_at, updated_at`
const agentQuotaLedgerColumns = `id, agent_id, product_id, duration_days, is_permanent, change_type, change_quantity, balance_after, related_batch_id, related_card_id, operator_type, operator_id, remark, created_at`
const auditColumns = `id, actor_type, actor_id, agent_id, product_id, license_id, card_id, binding_id, action, client_ip, user_agent, request_id, result, error_code, extra_json, created_at`

const auditInsertSQL = `
	INSERT INTO audit_logs (
	  id, actor_type, actor_id, agent_id, product_id, license_id, card_id, binding_id,
	  action, client_ip, user_agent, request_id, result, error_code, extra_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

type scanner interface {
	Scan(dest ...any) error
}

func scanProduct(row scanner) (domain.Product, error) {
	var p domain.Product
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.Name, &p.Code, &p.AppKey, &p.PublicKeyPEM, &p.PrivateKeyEncrypted,
		&p.KeyVersion,
		&p.BindMode, &p.MaxBindCount, &p.BindConflictStrategy, &p.OfflineMode,
		&p.OfflineGraceDays, &p.ExpireGraceDays, &p.UnbindLimit, &p.UnbindCooldownHours,
		&p.Status, &createdAt, &updatedAt)
	if err != nil {
		return domain.Product{}, err
	}
	p.CreatedAt = parseTS(createdAt)
	p.UpdatedAt = parseTS(updatedAt)
	return p, nil
}

func scanBatch(row scanner) (domain.CardBatch, error) {
	var b domain.CardBatch
	var agentID, createdBy sql.NullString
	var isPermanent int
	var createdAt, updatedAt string
	err := row.Scan(&b.ID, &b.ProductID, &agentID, &b.Name, &b.Quantity, &b.DurationDays,
		&isPermanent, &b.Source, &b.Status, &b.ExportCount, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		return domain.CardBatch{}, err
	}
	b.AgentID = str(agentID)
	b.CreatedBy = str(createdBy)
	b.IsPermanent = isPermanent == 1
	b.CreatedAt = parseTS(createdAt)
	b.UpdatedAt = parseTS(updatedAt)
	return b, nil
}

func scanCard(row scanner) (domain.Card, error) {
	var c domain.Card
	var agentID, orderNo, activatedLicenseID, voidReason, createdBy sql.NullString
	var activatedAt, consumedAt, voidedAt sql.NullString
	var createdAt, updatedAt string
	var isPermanent int
	err := row.Scan(&c.ID, &c.ProductID, &c.BatchID, &agentID, &c.CodeHash, &c.CodeEncrypted,
		&c.CodePrefix, &c.DurationDays, &isPermanent, &c.Status, &orderNo, &activatedLicenseID,
		&activatedAt, &consumedAt, &voidedAt, &voidReason, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		return domain.Card{}, err
	}
	c.AgentID = str(agentID)
	c.OrderNo = str(orderNo)
	c.ActivatedLicenseID = str(activatedLicenseID)
	c.VoidReason = str(voidReason)
	c.CreatedBy = str(createdBy)
	c.IsPermanent = isPermanent == 1
	c.ActivatedAt = timePtr(activatedAt)
	c.ConsumedAt = timePtr(consumedAt)
	c.VoidedAt = timePtr(voidedAt)
	c.CreatedAt = parseTS(createdAt)
	c.UpdatedAt = parseTS(updatedAt)
	return c, nil
}

func scanLicense(row scanner) (domain.License, error) {
	var l domain.License
	var agentID sql.NullString
	var expiredAt, lastVerifyAt, lastHeartbeatAt sql.NullString
	var issuedAt, activatedAt, createdAt, updatedAt string
	err := row.Scan(&l.ID, &l.LicenseNo, &l.ProductID, &l.CardID, &agentID, &l.Status,
		&issuedAt, &activatedAt, &expiredAt, &lastVerifyAt, &lastHeartbeatAt,
		&l.OfflineTokenVersion, &createdAt, &updatedAt)
	if err != nil {
		return domain.License{}, err
	}
	l.AgentID = str(agentID)
	l.IssuedAt = parseTS(issuedAt)
	l.ActivatedAt = parseTS(activatedAt)
	l.ExpiredAt = timePtr(expiredAt)
	l.LastVerifyAt = timePtr(lastVerifyAt)
	l.LastHeartbeatAt = timePtr(lastHeartbeatAt)
	l.CreatedAt = parseTS(createdAt)
	l.UpdatedAt = parseTS(updatedAt)
	return l, nil
}

func scanOfflineLicense(row scanner) (domain.OfflineLicense, error) {
	var license domain.OfflineLicense
	var label, expiredAt, revokedAt, revokedReason, createdBy sql.NullString
	var issuedAt, createdAt, updatedAt string
	err := row.Scan(&license.ID, &license.LicenseNo, &license.ProductID, &label,
		&license.MachineCodeHash, &license.MachineCodeEncrypted, &license.MachineCodeMasked,
		&license.SignedTokenEncrypted, &license.TokenVersion, &license.Status, &issuedAt,
		&expiredAt, &revokedAt, &revokedReason, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		return domain.OfflineLicense{}, err
	}
	license.Label = str(label)
	license.IssuedAt = parseTS(issuedAt)
	license.ExpiredAt = timePtr(expiredAt)
	license.RevokedAt = timePtr(revokedAt)
	license.RevokedReason = str(revokedReason)
	license.CreatedBy = str(createdBy)
	license.CreatedAt = parseTS(createdAt)
	license.UpdatedAt = parseTS(updatedAt)
	return license, nil
}

func scanBinding(row scanner) (domain.Binding, error) {
	var b domain.Binding
	var displayName, firstSeenIP, lastSeenIP, userAgent, lastHeartbeatAt sql.NullString
	var activatedAt, createdAt, updatedAt string
	err := row.Scan(&b.ID, &b.LicenseID, &b.ProductID, &b.BindMode, &b.BindValueHash,
		&b.BindValueEncrypted, &displayName, &b.Status, &firstSeenIP, &lastSeenIP,
		&userAgent, &lastHeartbeatAt, &activatedAt, &createdAt, &updatedAt)
	if err != nil {
		return domain.Binding{}, err
	}
	b.DisplayName = str(displayName)
	b.FirstSeenIP = str(firstSeenIP)
	b.LastSeenIP = str(lastSeenIP)
	b.UserAgent = str(userAgent)
	b.LastHeartbeatAt = timePtr(lastHeartbeatAt)
	b.ActivatedAt = parseTS(activatedAt)
	b.CreatedAt = parseTS(createdAt)
	b.UpdatedAt = parseTS(updatedAt)
	return b, nil
}

func scanAgent(row scanner) (domain.Agent, error) {
	var a domain.Agent
	var parentAgentID, contactName, phone, email, remark, createdBy, disabledAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&a.ID, &a.AgentNo, &parentAgentID, &a.Name, &contactName, &phone,
		&email, &a.Level, &a.Status, &a.SettlementMode, &a.DefaultDiscountRate,
		&a.CreditLimit, &remark, &createdBy, &createdAt, &updatedAt, &disabledAt)
	if err != nil {
		return domain.Agent{}, err
	}
	a.ParentAgentID = str(parentAgentID)
	a.ContactName = str(contactName)
	a.Phone = str(phone)
	a.Email = str(email)
	a.Remark = str(remark)
	a.CreatedBy = str(createdBy)
	a.CreatedAt = parseTS(createdAt)
	a.UpdatedAt = parseTS(updatedAt)
	a.DisabledAt = timePtr(disabledAt)
	return a, nil
}

func scanAgentUser(row scanner) (domain.AgentUser, error) {
	var u domain.AgentUser
	var displayName, phone, email, lastLoginAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&u.ID, &u.AgentID, &u.Username, &u.PasswordHash, &displayName,
		&phone, &email, &u.Role, &u.Status, &lastLoginAt, &createdAt, &updatedAt)
	if err != nil {
		return domain.AgentUser{}, err
	}
	u.DisplayName = str(displayName)
	u.Phone = str(phone)
	u.Email = str(email)
	u.LastLoginAt = timePtr(lastLoginAt)
	u.CreatedAt = parseTS(createdAt)
	u.UpdatedAt = parseTS(updatedAt)
	return u, nil
}

func scanAdminUser(row scanner) (domain.AdminUser, error) {
	var u domain.AdminUser
	var displayName, lastLoginAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &displayName, &u.Role,
		&u.Status, &u.SessionVersion, &lastLoginAt, &createdAt, &updatedAt)
	if err != nil {
		return domain.AdminUser{}, err
	}
	u.DisplayName = str(displayName)
	u.LastLoginAt = timePtr(lastLoginAt)
	u.CreatedAt = parseTS(createdAt)
	u.UpdatedAt = parseTS(updatedAt)
	return u, nil
}

func scanAgentProductPolicy(row scanner) (domain.AgentProductPolicy, error) {
	var p domain.AgentProductPolicy
	var canGenerate, canExport, allowPermanent int
	var durations string
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.AgentID, &p.ProductID, &canGenerate, &canExport,
		&durations, &allowPermanent, &p.MaxBatchQuantity, &p.DiscountRate,
		&p.SettlementPrice, &p.Status, &createdAt, &updatedAt)
	if err != nil {
		return domain.AgentProductPolicy{}, err
	}
	p.CanGenerate = canGenerate == 1
	p.CanExportPlainCode = canExport == 1
	p.AllowPermanent = allowPermanent == 1
	if durations != "" {
		if err := json.Unmarshal([]byte(durations), &p.AllowedDurationDays); err != nil {
			return domain.AgentProductPolicy{}, err
		}
	}
	p.CreatedAt = parseTS(createdAt)
	p.UpdatedAt = parseTS(updatedAt)
	return p, nil
}

func scanAgentQuotaLedger(row scanner) (domain.AgentQuotaLedger, error) {
	var l domain.AgentQuotaLedger
	var isPermanent int
	var relatedBatchID, relatedCardID, operatorID, remark sql.NullString
	var createdAt string
	err := row.Scan(&l.ID, &l.AgentID, &l.ProductID, &l.DurationDays, &isPermanent,
		&l.ChangeType, &l.ChangeQuantity, &l.BalanceAfter, &relatedBatchID,
		&relatedCardID, &l.OperatorType, &operatorID, &remark, &createdAt)
	if err != nil {
		return domain.AgentQuotaLedger{}, err
	}
	l.IsPermanent = isPermanent == 1
	l.RelatedBatchID = str(relatedBatchID)
	l.RelatedCardID = str(relatedCardID)
	l.OperatorID = str(operatorID)
	l.Remark = str(remark)
	l.CreatedAt = parseTS(createdAt)
	return l, nil
}

func scanAuditLog(row scanner) (domain.AuditLog, error) {
	var log domain.AuditLog
	var actorID, agentID, productID, licenseID, cardID, bindingID sql.NullString
	var clientIP, userAgent, requestID, errorCode, extraJSON sql.NullString
	var createdAt string
	err := row.Scan(&log.ID, &log.ActorType, &actorID, &agentID, &productID, &licenseID,
		&cardID, &bindingID, &log.Action, &clientIP, &userAgent, &requestID, &log.Result,
		&errorCode, &extraJSON, &createdAt)
	if err != nil {
		return domain.AuditLog{}, err
	}
	log.ActorID = str(actorID)
	log.AgentID = str(agentID)
	log.ProductID = str(productID)
	log.LicenseID = str(licenseID)
	log.CardID = str(cardID)
	log.BindingID = str(bindingID)
	log.ClientIP = str(clientIP)
	log.UserAgent = str(userAgent)
	log.RequestID = str(requestID)
	log.ErrorCode = str(errorCode)
	log.ExtraJSON = str(extraJSON)
	log.CreatedAt = parseTS(createdAt)
	return log, nil
}

func ts(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTS(value string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: ts(*value), Valid: true}
}

func timePtr(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}
	t := parseTS(value.String)
	return &t
}

func str(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func requireAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

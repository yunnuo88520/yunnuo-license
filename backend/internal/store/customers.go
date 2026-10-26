package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

const customerColumns = `id,username,email,password_hash,display_name,status,session_version,created_at`

func scanCustomer(row scanner) (domain.Customer, error) {
	var c domain.Customer
	var created string
	err := row.Scan(&c.ID, &c.Username, &c.Email, &c.PasswordHash, &c.DisplayName, &c.Status, &c.SessionVersion, &created)
	c.CreatedAt = parseTS(created)
	return c, err
}
func (s *Store) CustomerByUsername(ctx context.Context, name string) (domain.Customer, error) {
	return scanCustomer(s.db.QueryRowContext(ctx, `SELECT `+customerColumns+` FROM customer_users WHERE username=?`, name))
}
func (s *Store) CustomerByID(ctx context.Context, id string) (domain.Customer, error) {
	return scanCustomer(s.db.QueryRowContext(ctx, `SELECT `+customerColumns+` FROM customer_users WHERE id=?`, id))
}
func (s *Store) Customers(ctx context.Context) ([]domain.Customer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.username,c.email,c.password_hash,c.display_name,c.status,c.session_version,c.created_at,
		COALESCE(m.agent_id,''),COALESCE(CASE WHEN m.status='active' THEN a.status ELSE 'disabled' END,'')
		FROM customer_users c LEFT JOIN agent_user_identities i ON i.user_id=c.id
		LEFT JOIN agent_users m ON m.id=i.member_id LEFT JOIN agents a ON a.id=m.agent_id ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Customer{}
	for rows.Next() {
		var c domain.Customer
		var created string
		err := rows.Scan(&c.ID, &c.Username, &c.Email, &c.PasswordHash, &c.DisplayName, &c.Status, &c.SessionVersion, &created, &c.AgentID, &c.AgentStatus)
		if err != nil {
			return nil, err
		}
		c.CreatedAt = parseTS(created)
		if c.AgentID == "" {
			c.AgentStatus = ""
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
func (tx *Tx) CreateCustomer(ctx context.Context, c domain.Customer) error {
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO customer_users (`+customerColumns+`) VALUES (?,?,?,?,?,?,?,?)`, c.ID, c.Username, c.Email, c.PasswordHash, c.DisplayName, c.Status, c.SessionVersion, ts(c.CreatedAt))
	return err
}
func (tx *Tx) SetCustomerStatus(ctx context.Context, id, status string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE customer_users SET status=?, session_version=session_version+1 WHERE id=?`, status, id)
	return err
}
func (tx *Tx) SetCustomerPassword(ctx context.Context, id, hash string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE customer_users SET password_hash=?,session_version=session_version+1 WHERE id=?`, hash, id)
	if err != nil {
		return err
	}
	return tx.syncAgentIdentityPassword(ctx, id, hash)
}
func (tx *Tx) LockLicense(ctx context.Context, id string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE licenses SET updated_at=updated_at WHERE id=?`, id)
	return err
}
func (tx *Tx) Owner(ctx context.Context, licenseID string) (string, error) {
	var owner string
	err := tx.tx.QueryRowContext(ctx, `SELECT user_id FROM license_owners WHERE license_id=?`, licenseID).Scan(&owner)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return owner, err
}
func (tx *Tx) SetOwner(ctx context.Context, licenseID, userID string, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM license_owners WHERE license_id=?`, licenseID); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO license_owners (license_id,user_id,assigned_at) VALUES (?,?,?)`, licenseID, userID, ts(now))
	return err
}
func (s *Store) CustomerLicenses(ctx context.Context, userID string) ([]domain.License, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+licenseColumns+` FROM licenses WHERE id IN (SELECT license_id FROM license_owners WHERE user_id=?) ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.License{}
	for rows.Next() {
		lic, err := scanLicense(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, lic)
	}
	return items, rows.Err()
}
func (s *Store) ProductByCode(ctx context.Context, code string) (domain.Product, error) {
	return scanProduct(s.db.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE code=?`, code))
}

func (tx *Tx) BindingPolicy(ctx context.Context, p domain.Product) (domain.BindingPolicy, error) {
	policy := domain.BindingPolicy{ProductID: p.ID, BindMode: p.BindMode, Allowed: p.UnbindLimit != 0, Limit: p.UnbindLimit, CooldownHours: p.UnbindCooldownHours}
	err := tx.tx.QueryRowContext(ctx, `SELECT allowed,change_limit,cooldown_hours,allow_owner_transfer FROM product_binding_policies WHERE product_id=? AND bind_mode=?`, p.ID, p.BindMode).Scan(&policy.Allowed, &policy.Limit, &policy.CooldownHours, &policy.AllowOwnerTransfer)
	if err == sql.ErrNoRows {
		return policy, nil
	}
	return policy, err
}
func (s *Store) BindingPolicy(ctx context.Context, p domain.Product) (domain.BindingPolicy, error) {
	var policy domain.BindingPolicy
	err := s.WithTx(ctx, func(tx *Tx) error { var err error; policy, err = tx.BindingPolicy(ctx, p); return err })
	return policy, err
}
func (tx *Tx) SetBindingPolicy(ctx context.Context, p domain.BindingPolicy) error {
	if err := tx.lockProduct(ctx, p.ProductID); err != nil {
		return err
	}
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM product_binding_policies WHERE product_id=? AND bind_mode=?`, p.ProductID, p.BindMode); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO product_binding_policies (product_id,bind_mode,allowed,change_limit,cooldown_hours,allow_owner_transfer) VALUES (?,?,?,?,?,?)`, p.ProductID, p.BindMode, p.Allowed, p.Limit, p.CooldownHours, p.AllowOwnerTransfer)
	return err
}
func (tx *Tx) BindingChangeStats(ctx context.Context, licenseID string) (int, *time.Time, error) {
	var count int
	var last sql.NullString
	err := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*),MAX(created_at) FROM binding_changes WHERE license_id=?`, licenseID).Scan(&count, &last)
	return count, timePtr(last), err
}
func (s *Store) BindingChangeStats(ctx context.Context, id string) (int, *time.Time, error) {
	var count int
	var last *time.Time
	err := s.WithTx(ctx, func(tx *Tx) error { var err error; count, last, err = tx.BindingChangeStats(ctx, id); return err })
	return count, last, err
}
func (tx *Tx) BindingChange(ctx context.Context, id, requestID string) (string, error) {
	var hash string
	err := tx.tx.QueryRowContext(ctx, `SELECT value_hash FROM binding_changes WHERE license_id=? AND request_id=?`, id, requestID).Scan(&hash)
	return hash, err
}
func (tx *Tx) AddBindingChange(ctx context.Context, id, licenseID, bindingID, requestID, hash, kind string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO binding_changes (id,license_id,binding_id,request_id,value_hash,kind,created_at) VALUES (?,?,?,?,?,?,?)`, id, licenseID, bindingID, requestID, hash, kind, ts(now))
	return err
}
func (tx *Tx) ReplaceBinding(ctx context.Context, id, hash, encrypted string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE license_bindings SET bind_value_hash=?,bind_value_encrypted=?,updated_at=? WHERE id=? AND status=?`, hash, encrypted, ts(now), id, domain.BindingActive)
	return err
}

package store

import (
	"context"
	"encoding/json"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"strings"
	"time"
)

func decodeTerms(raw string) (*domain.AuthorizationTerms, error) {
	if raw == "" {
		return nil, nil
	}
	var t domain.AuthorizationTerms
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return nil, err
	}
	return &t, nil
}
func (tx *Tx) SaveTerms(ctx context.Context, scope, id string, t *domain.AuthorizationTerms) error {
	if t == nil {
		return nil
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if _, err = tx.tx.ExecContext(ctx, `DELETE FROM authorization_snapshots WHERE scope=? AND resource_id=?`, scope, id); err != nil {
		return err
	}
	_, err = tx.tx.ExecContext(ctx, `INSERT INTO authorization_snapshots(scope,resource_id,terms_json) VALUES(?,?,?)`, scope, id, string(raw))
	return err
}

func (tx *Tx) IndexBindingSubject(ctx context.Context, id, kind, hash string) error {
	_, e := tx.tx.ExecContext(ctx, `INSERT INTO binding_subjects(binding_id,kind,subject_hash) VALUES(?,?,?)`, id, kind, hash)
	return e
}
func (tx *Tx) ClearBindingSubjects(ctx context.Context, id string) error {
	_, e := tx.tx.ExecContext(ctx, `DELETE FROM binding_subjects WHERE binding_id=?`, id)
	return e
}
func (s *Store) LookupBindingSubjects(ctx context.Context, kind string, hashes []string) ([]domain.Binding, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	args := []any{kind}
	placeholders := make([]string, len(hashes))
	for i, h := range hashes {
		args = append(args, h)
		placeholders[i] = "?"
	}
	rows, e := s.db.QueryContext(ctx, `SELECT `+bindingColumns+` FROM license_bindings WHERE status='active' AND id IN (SELECT binding_id FROM binding_subjects WHERE kind=? AND subject_hash IN (`+strings.Join(placeholders, ",")+`)) ORDER BY activated_at DESC LIMIT 20`, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []domain.Binding{}
	for rows.Next() {
		v, e := scanBinding(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (s *Store) SaveProductTerms(ctx context.Context, id string, t domain.AuthorizationTerms) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockProduct(ctx, id); err != nil {
			return err
		}
		return tx.SaveTerms(ctx, "product", id, &t)
	})
}
func (tx *Tx) GetBindings(ctx context.Context, id string) ([]domain.Binding, error) {
	rows, err := tx.tx.QueryContext(ctx, `SELECT `+bindingColumns+` FROM license_bindings WHERE license_id=? ORDER BY created_at,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Binding{}
	for rows.Next() {
		v, e := scanBinding(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (tx *Tx) Installation(ctx context.Context, license, id string) (domain.Installation, error) {
	var v domain.Installation
	var seen, created string
	err := tx.tx.QueryRowContext(ctx, `SELECT license_id,instance_id,binding_id,secret_hash,device_hash,label,status,last_seen_at,created_at FROM license_installations WHERE license_id=? AND instance_id=?`, license, id).Scan(&v.LicenseID, &v.ID, &v.BindingID, &v.SecretHash, &v.DeviceHash, &v.Label, &v.Status, &seen, &created)
	v.LastSeenAt, v.CreatedAt = parseTS(seen), parseTS(created)
	return v, err
}
func (tx *Tx) CreateInstallation(ctx context.Context, v domain.Installation) error {
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO license_installations(license_id,instance_id,binding_id,secret_hash,device_hash,label,status,last_seen_at,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, v.LicenseID, v.ID, v.BindingID, v.SecretHash, v.DeviceHash, v.Label, v.Status, ts(v.LastSeenAt), ts(v.CreatedAt))
	return err
}
func (tx *Tx) TouchInstallation(ctx context.Context, license, id string, now time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE license_installations SET last_seen_at=? WHERE license_id=? AND instance_id=?`, ts(now), license, id)
	return err
}
func (tx *Tx) InstallationCounts(ctx context.Context, license, device string) (count, devices, exists int, err error) {
	err = tx.tx.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(DISTINCT device_hash),COALESCE(SUM(CASE WHEN device_hash=? THEN 1 ELSE 0 END),0) FROM license_installations WHERE license_id=? AND status='active' AND binding_id IN (SELECT id FROM license_bindings WHERE license_id=? AND status='active')`, device, license, license).Scan(&count, &devices, &exists)
	return
}
func (s *Store) Installations(ctx context.Context, license string) ([]domain.Installation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT instance_id,binding_id,label,status,last_seen_at,created_at FROM license_installations WHERE license_id=? ORDER BY created_at,instance_id`, license)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Installation{}
	for rows.Next() {
		var v domain.Installation
		var seen, created string
		if err = rows.Scan(&v.ID, &v.BindingID, &v.Label, &v.Status, &seen, &created); err != nil {
			return nil, err
		}
		v.LastSeenAt, v.CreatedAt = parseTS(seen), parseTS(created)
		items = append(items, v)
	}
	return items, rows.Err()
}
func (tx *Tx) DisableInstallation(ctx context.Context, license, id string) error {
	r, e := tx.tx.ExecContext(ctx, `UPDATE license_installations SET status='disabled' WHERE license_id=? AND instance_id=? AND status='active'`, license, id)
	if e != nil {
		return e
	}
	if e = requireAffected(r); e != nil {
		return e
	}
	_, e = tx.tx.ExecContext(ctx, `DELETE FROM license_seat_leases WHERE license_id=? AND instance_id=? AND borrowed=0`, license, id)
	return e
}
func leaseTS(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }
func (tx *Tx) Seats(ctx context.Context, license string, now time.Time) ([]domain.SeatLease, error) {
	rows, err := tx.tx.QueryContext(ctx, `SELECT id,instance_id,session_id,expires_at,borrowed FROM license_seat_leases WHERE license_id=? AND expires_at>?`, license, leaseTS(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.SeatLease{}
	for rows.Next() {
		var v domain.SeatLease
		var expiry string
		if err = rows.Scan(&v.ID, &v.InstanceID, &v.SessionID, &expiry, &v.Borrowed); err != nil {
			return nil, err
		}
		v.LicenseID = license
		v.ExpiresAt = parseTS(expiry)
		items = append(items, v)
	}
	return items, rows.Err()
}
func (tx *Tx) SaveSeat(ctx context.Context, v domain.SeatLease) error {
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM license_seat_leases WHERE license_id=? AND instance_id=? AND session_id=?`, v.LicenseID, v.InstanceID, v.SessionID); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO license_seat_leases(id,license_id,instance_id,session_id,expires_at,borrowed) VALUES(?,?,?,?,?,?)`, v.ID, v.LicenseID, v.InstanceID, v.SessionID, leaseTS(v.ExpiresAt), boolInt(v.Borrowed))
	return err
}
func (tx *Tx) ReleaseSeat(ctx context.Context, license, instance, id, session string) error {
	_, err := tx.tx.ExecContext(ctx, `DELETE FROM license_seat_leases WHERE license_id=? AND instance_id=? AND id=? AND session_id=? AND borrowed=0`, license, instance, id, session)
	return err
}
func (tx *Tx) CreateClaimTicket(ctx context.Context, hash, license string, until time.Time) error {
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO license_claim_tickets(token_hash,license_id,expires_at,claimed_by) VALUES(?,?,?,'')`, hash, license, leaseTS(until))
	return err
}
func (tx *Tx) ClaimTicket(ctx context.Context, hash string) (license, owner string, expiry time.Time, err error) {
	// The no-op write serializes concurrent redemption before reading its state.
	if _, err = tx.tx.ExecContext(ctx, `UPDATE license_claim_tickets SET token_hash=token_hash WHERE token_hash=?`, hash); err != nil {
		return
	}
	var raw string
	err = tx.tx.QueryRowContext(ctx, `SELECT license_id,claimed_by,expires_at FROM license_claim_tickets WHERE token_hash=?`, hash).Scan(&license, &owner, &raw)
	expiry = parseTS(raw)
	return
}
func (tx *Tx) ConsumeClaimTicket(ctx context.Context, hash, user string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE license_claim_tickets SET claimed_by=? WHERE token_hash=?`, user, hash)
	return err
}

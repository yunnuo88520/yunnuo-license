package store

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func readSecurityPolicy(ctx context.Context, q commerceQuery, product string) (domain.SecurityPolicy, error) {
	p := domain.SecurityPolicy{ProductID: product, Action: "observe", Message: "此应用的授权需要核实，请联系服务提供方。"}
	var enabled, mismatch, reports int
	err := q.QueryRowContext(ctx, `SELECT enabled,detect_mismatch,accept_reports,action,message,support_url FROM product_security_policies WHERE product_id=?`, product).Scan(&enabled, &mismatch, &reports, &p.Action, &p.Message, &p.SupportURL)
	p.Enabled = enabled == 1
	p.DetectMismatch = mismatch == 1
	p.AcceptReports = reports == 1
	if IsNotFound(err) {
		err = nil
	}
	return p, err
}
func (s *Store) SecurityPolicy(ctx context.Context, id string) (domain.SecurityPolicy, error) {
	return readSecurityPolicy(ctx, s.db, id)
}
func (tx *Tx) SecurityPolicy(ctx context.Context, id string) (domain.SecurityPolicy, error) {
	return readSecurityPolicy(ctx, tx.tx, id)
}
func (tx *Tx) SaveSecurityPolicy(ctx context.Context, p domain.SecurityPolicy) error {
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM product_security_policies WHERE product_id=?`, p.ProductID); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO product_security_policies (product_id,enabled,detect_mismatch,accept_reports,action,message,support_url) VALUES (?,?,?,?,?,?,?)`, p.ProductID, boolInt(p.Enabled), boolInt(p.DetectMismatch), boolInt(p.AcceptReports), p.Action, p.Message, p.SupportURL)
	return err
}

const caseColumns = `c.id,c.product_id,c.license_id,l.license_no,c.signal_type,c.subject,c.evidence,c.status,c.action,c.occurrences,c.first_seen,c.last_seen,c.review_reason,c.reviewer`

func readSecurityCase(row scanner) (domain.SecurityCase, error) {
	var c domain.SecurityCase
	var first, last string
	err := row.Scan(&c.ID, &c.ProductID, &c.LicenseID, &c.LicenseNo, &c.Signal, &c.Subject, &c.Evidence, &c.Status, &c.Action, &c.Occurrences, &first, &last, &c.ReviewReason, &c.Reviewer)
	c.FirstSeen = parseTS(first)
	c.LastSeen = parseTS(last)
	return c, err
}
func (tx *Tx) SecurityCase(ctx context.Context, id string) (domain.SecurityCase, error) {
	return readSecurityCase(tx.tx.QueryRowContext(ctx, `SELECT `+caseColumns+` FROM security_cases c JOIN licenses l ON l.id=c.license_id WHERE c.id=?`, id))
}
func (s *Store) SecurityCase(ctx context.Context, id string) (domain.SecurityCase, error) {
	return readSecurityCase(s.db.QueryRowContext(ctx, `SELECT `+caseColumns+` FROM security_cases c JOIN licenses l ON l.id=c.license_id WHERE c.id=?`, id))
}
func (tx *Tx) RecordSecurityCase(ctx context.Context, c domain.SecurityCase) (domain.SecurityCase, error) {
	existing, err := readSecurityCase(tx.tx.QueryRowContext(ctx, `SELECT `+caseColumns+` FROM security_cases c JOIN licenses l ON l.id=c.license_id WHERE c.product_id=? AND c.license_id=? AND c.signal_type=?`, c.ProductID, c.LicenseID, c.Signal))
	if err == nil {
		_, err = tx.tx.ExecContext(ctx, `UPDATE security_cases SET occurrences=occurrences+1,last_seen=? WHERE id=?`, ts(c.LastSeen), existing.ID)
		if err != nil {
			return c, err
		}
		return tx.SecurityCase(ctx, existing.ID)
	}
	if !IsNotFound(err) {
		return c, err
	}
	_, err = tx.tx.ExecContext(ctx, `INSERT INTO security_cases (id,product_id,license_id,signal_type,subject,evidence,status,action,occurrences,first_seen,last_seen,review_reason,reviewer) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, c.ProductID, c.LicenseID, c.Signal, c.Subject, c.Evidence, c.Status, c.Action, c.Occurrences, ts(c.FirstSeen), ts(c.LastSeen), c.ReviewReason, c.Reviewer)
	return c, err
}
func securityCases(ctx context.Context, q commerceQuery, user string) ([]domain.SecurityCase, error) {
	where := ""
	args := []any{}
	if user != "" {
		where = ` WHERE c.license_id IN (SELECT license_id FROM license_owners WHERE user_id=?)`
		args = append(args, user)
	}
	rows, err := q.QueryContext(ctx, `SELECT `+caseColumns+` FROM security_cases c JOIN licenses l ON l.id=c.license_id`+where+` ORDER BY c.last_seen DESC,c.id DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.SecurityCase{}
	for rows.Next() {
		c, e := readSecurityCase(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
func (s *Store) SecurityCases(ctx context.Context, user string) ([]domain.SecurityCase, error) {
	return securityCases(ctx, s.db, user)
}
func activeSecurityCase(ctx context.Context, q commerceQuery, license string) (domain.SecurityCase, error) {
	return readSecurityCase(q.QueryRowContext(ctx, `SELECT `+caseColumns+` FROM security_cases c JOIN licenses l ON l.id=c.license_id WHERE c.license_id=? AND c.status='confirmed' ORDER BY CASE c.action WHEN 'block_page' THEN 4 WHEN 'deny_use' THEN 3 WHEN 'deny_updates' THEN 2 ELSE 1 END DESC,c.id LIMIT 1`, license))
}
func (s *Store) ActiveSecurityCase(ctx context.Context, id string) (domain.SecurityCase, error) {
	return activeSecurityCase(ctx, s.db, id)
}
func (tx *Tx) ActiveSecurityCase(ctx context.Context, id string) (domain.SecurityCase, error) {
	return activeSecurityCase(ctx, tx.tx, id)
}
func (tx *Tx) ReviewSecurityCase(ctx context.Context, id, status, action, reason, actor string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE security_cases SET status=?,action=?,review_reason=?,reviewer=? WHERE id=?`, status, action, reason, actor, id)
	if err != nil {
		return err
	}
	if status == "resolved" {
		_, err = tx.tx.ExecContext(ctx, `UPDATE security_appeals SET status='closed' WHERE case_id=?`, id)
	}
	return err
}
func (tx *Tx) Appeal(ctx context.Context, a domain.SecurityAppeal) (domain.SecurityAppeal, error) {
	existing, err := readAppeal(tx.tx.QueryRowContext(ctx, `SELECT id,case_id,user_id,reason,status,created_at FROM security_appeals WHERE case_id=? AND user_id=?`, a.CaseID, a.UserID))
	if err == nil {
		return existing, nil
	}
	if !IsNotFound(err) {
		return a, err
	}
	_, err = tx.tx.ExecContext(ctx, `INSERT INTO security_appeals (id,case_id,user_id,reason,status,created_at) VALUES (?,?,?,?,?,?)`, a.ID, a.CaseID, a.UserID, a.Reason, a.Status, ts(a.CreatedAt))
	return a, err
}
func readAppeal(row scanner) (domain.SecurityAppeal, error) {
	var a domain.SecurityAppeal
	var created string
	err := row.Scan(&a.ID, &a.CaseID, &a.UserID, &a.Reason, &a.Status, &created)
	a.CreatedAt = parseTS(created)
	return a, err
}
func (s *Store) SecurityAppeals(ctx context.Context, user string) ([]domain.SecurityAppeal, error) {
	where := ""
	args := []any{}
	if user != "" {
		where = " WHERE user_id=?"
		args = append(args, user)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,case_id,user_id,reason,status,created_at FROM security_appeals`+where+` ORDER BY created_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.SecurityAppeal{}
	for rows.Next() {
		a, e := readAppeal(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

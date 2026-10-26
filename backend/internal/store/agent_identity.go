package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func (s *Store) CustomerByLogin(ctx context.Context, login string) (domain.Customer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+customerColumns+` FROM customer_users WHERE username=? OR (email<>'' AND LOWER(email)=?)`, login, strings.ToLower(login))
	if err != nil {
		return domain.Customer{}, err
	}
	defer rows.Close()
	var result domain.Customer
	count := 0
	for rows.Next() {
		value, e := scanCustomer(rows)
		if e != nil {
			return result, e
		}
		result = value
		count++
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if count != 1 {
		return domain.Customer{}, sql.ErrNoRows
	}
	return result, nil
}
func (tx *Tx) CustomerByID(ctx context.Context, id string) (domain.Customer, error) {
	return scanCustomer(tx.tx.QueryRowContext(ctx, `SELECT `+customerColumns+` FROM customer_users WHERE id=?`, id))
}
func (tx *Tx) BindAgentIdentity(ctx context.Context, member, user string) error {
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO agent_user_identities(member_id,user_id) VALUES(?,?)`, member, user)
	return err
}
func (s *Store) AgentMemberForUser(ctx context.Context, user string) (domain.AgentUser, error) {
	return scanAgentUser(s.db.QueryRowContext(ctx, `SELECT `+agentUserColumns+` FROM agent_users WHERE id=(SELECT member_id FROM agent_user_identities WHERE user_id=?)`, user))
}
func (tx *Tx) AgentMemberForUser(ctx context.Context, user string) (domain.AgentUser, error) {
	return scanAgentUser(tx.tx.QueryRowContext(ctx, `SELECT `+agentUserColumns+` FROM agent_users WHERE id=(SELECT member_id FROM agent_user_identities WHERE user_id=?)`, user))
}
func (s *Store) AgentIdentityID(ctx context.Context, member string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM agent_user_identities WHERE member_id=?`, member).Scan(&id)
	return id, err
}
func (s *Store) EligibleAgentUsers(ctx context.Context) ([]domain.Customer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+customerColumns+` FROM customer_users WHERE status='active' AND NOT EXISTS(SELECT 1 FROM agent_user_identities i WHERE i.user_id=customer_users.id) ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Customer{}
	for rows.Next() {
		c, e := scanCustomer(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (tx *Tx) IdentityLoginAvailable(ctx context.Context, username, email string) (bool, error) {
	// Serialize cross-column username/email checks on both SQLite and MySQL.
	// This must precede the first consistent read in an account-creation transaction.
	if _, err := tx.tx.ExecContext(ctx, `UPDATE account_identity_lock SET id=id WHERE id=1`); err != nil {
		return false, err
	}
	var count int
	err := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM customer_users WHERE username=? OR (email<>'' AND LOWER(email)=?) OR (?<>'' AND (LOWER(username)=? OR LOWER(email)=?))`, username, username, email, email, email).Scan(&count)
	return count == 0, err
}

// Existing memberships become independent login identities. A conflicting name
// never merges accounts: it receives an explicit unique login, retaining its password.
func (s *Store) ensureAgentIdentities(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentUserColumns+` FROM agent_users WHERE NOT EXISTS(SELECT 1 FROM agent_user_identities i WHERE i.member_id=agent_users.id) ORDER BY created_at,id`)
	if err != nil {
		return err
	}
	members := []domain.AgentUser{}
	for rows.Next() {
		m, e := scanAgentUser(rows)
		if e != nil {
			rows.Close()
			return e
		}
		members = append(members, m)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, m := range members {
		err = s.WithTx(ctx, func(tx *Tx) error {
			name := m.Username
			email := strings.ToLower(strings.TrimSpace(m.Email))
			available, e := tx.IdentityLoginAvailable(ctx, name, "")
			if e != nil {
				return e
			}
			if !available {
				name = "agent-" + strings.TrimPrefix(m.ID, "aguser_")
			}
			if email != "" {
				ok, e := tx.IdentityLoginAvailable(ctx, name, email)
				if e != nil {
					return e
				}
				if !ok {
					email = ""
				}
			}
			id := yncrypto.NewID("usr")
			c := domain.Customer{ID: id, Username: name, Email: email, PasswordHash: m.PasswordHash, DisplayName: m.DisplayName, Status: m.Status, SessionVersion: 1, CreatedAt: m.CreatedAt}
			if e = tx.CreateCustomer(ctx, c); e != nil {
				return fmt.Errorf("migrate agent login %s: %w", m.ID, e)
			}
			if _, e = tx.tx.ExecContext(ctx, `UPDATE agent_users SET username=? WHERE id=?`, name, m.ID); e != nil {
				return e
			}
			return tx.BindAgentIdentity(ctx, m.ID, c.ID)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (tx *Tx) syncAgentIdentityPassword(ctx context.Context, user, hash string) error {
	var member string
	err := tx.tx.QueryRowContext(ctx, `SELECT member_id FROM agent_user_identities WHERE user_id=?`, user).Scan(&member)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.tx.ExecContext(ctx, `UPDATE agent_users SET password_hash=? WHERE id=?`, hash, member); err != nil {
		return err
	}
	var version int
	err = tx.tx.QueryRowContext(ctx, `SELECT version FROM agent_session_versions WHERE user_id=?`, member).Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err = tx.tx.ExecContext(ctx, `DELETE FROM agent_session_versions WHERE user_id=?`, member); err != nil {
		return err
	}
	_, err = tx.tx.ExecContext(ctx, `INSERT INTO agent_session_versions(user_id,version) VALUES(?,?)`, member, version+1)
	return err
}

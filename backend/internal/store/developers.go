package store

import (
	"context"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func (s *Store) ListDeveloperProducts(ctx context.Context, userID string) ([]domain.Product, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+productColumns+` FROM products WHERE id IN (SELECT product_id FROM product_developers WHERE user_id = ?) ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

func (s *Store) AssignDeveloper(ctx context.Context, productID, userID string) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		if _, err := tx.tx.ExecContext(ctx, `DELETE FROM product_developers WHERE product_id = ?`, productID); err != nil {
			return err
		}
		if userID == "" {
			return nil
		}
		_, err := tx.tx.ExecContext(ctx, `INSERT INTO product_developers (product_id, user_id) VALUES (?, ?)`, productID, userID)
		return err
	})
}

func (s *Store) SetDeveloperStatus(ctx context.Context, userID, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE admin_users SET status = ?, session_version = session_version + 1 WHERE id = ? AND role = ?`, status, userID, domain.AdminRoleDeveloper)
	return err
}

package store

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func (s *Store) BackupReleases(ctx context.Context) ([]domain.ProductRelease, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+releaseColumns+` FROM product_releases WHERE status<>'deleted' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ProductRelease
	for rows.Next() {
		r, e := scanRelease(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

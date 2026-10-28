package store

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
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
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	// The maintenance snapshot is an explicitly privileged cross-application
	// reader. Older snapshots may not have the new schema yet.
	var exists int
	if s.driver == "mysql" {
		err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='application_releases'").Scan(&exists)
	} else {
		err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='application_releases'").Scan(&exists)
	}
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		return out, nil
	}
	scopedRows, err := s.db.QueryContext(ctx, "SELECT application_id,id,data_json FROM application_releases WHERE status<>'deleted' ORDER BY application_id,id")
	if err != nil {
		return nil, err
	}
	defer scopedRows.Close()
	for scopedRows.Next() {
		var app, id, raw string
		if err = scopedRows.Scan(&app, &id, &raw); err != nil {
			return nil, err
		}
		r, e := unmarshalCoreRecord[core.Release](raw)
		if e != nil {
			return nil, e
		}
		if r.ApplicationID != app || r.ID != id {
			return nil, core.ErrScopeMismatch
		}
		out = append(out, domain.ProductRelease{ID: r.ID, ProductID: app, Version: r.Version, Filename: r.Filename, Size: r.Size, SHA256: r.SHA256, Status: r.Status})
	}
	return out, scopedRows.Err()
}

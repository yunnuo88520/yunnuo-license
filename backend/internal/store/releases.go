package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func (s *Store) ProductUpdates(ctx context.Context, productID string) (domain.ProductUpdateSettings, error) {
	settings := domain.ProductUpdateSettings{ProductID: productID}
	err := s.db.QueryRowContext(ctx, `SELECT enabled FROM product_update_settings WHERE product_id = ?`, productID).Scan(&settings.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	return settings, err
}

func (s *Store) SetProductUpdates(ctx context.Context, settings domain.ProductUpdateSettings) error {
	query := `INSERT INTO product_update_settings (product_id, enabled) VALUES (?, ?) ON CONFLICT(product_id) DO UPDATE SET enabled = excluded.enabled`
	if s.driver == "mysql" {
		query = `INSERT INTO product_update_settings (product_id, enabled) VALUES (?, ?) ON DUPLICATE KEY UPDATE enabled = VALUES(enabled)`
	}
	_, err := s.db.ExecContext(ctx, query, settings.ProductID, settings.Enabled)
	return err
}

const releaseWriteColumns = `id, product_id, version, notes, status, filename, size, sha256, key_version, manifest_token, created_at`
const releaseColumns = releaseWriteColumns + `,COALESCE((SELECT target_json FROM release_targets WHERE release_id=product_releases.id),''),COALESCE((SELECT version FROM release_targets WHERE release_id=product_releases.id),version)`

func scanRelease(row interface{ Scan(...any) error }) (domain.ProductRelease, error) {
	var r domain.ProductRelease
	var created, target, version string
	err := row.Scan(&r.ID, &r.ProductID, &r.Version, &r.Notes, &r.Status, &r.Filename, &r.Size, &r.SHA256, &r.KeyVersion, &r.ManifestToken, &created, &target, &version)
	if err != nil {
		return r, err
	}
	if target != "" {
		r.Target = &domain.ReleaseTarget{}
		if err = json.Unmarshal([]byte(target), r.Target); err != nil {
			return r, err
		}
	}
	r.Version = version
	r.CreatedAt = parseTS(created)
	return r, err
}

func (s *Store) CreateRelease(ctx context.Context, r domain.ProductRelease) error {
	return s.WithTx(ctx, func(tx *Tx) error { return tx.CreateRelease(ctx, r) })
}

func (tx *Tx) saveReleaseTarget(ctx context.Context, r domain.ProductRelease) error {
	if r.Target == nil {
		return nil
	}
	raw, err := json.Marshal(r.Target)
	if err != nil {
		return err
	}
	_, err = tx.tx.ExecContext(ctx, `INSERT INTO release_targets(release_id,version,target_json) VALUES(?,?,?)`, r.ID, r.Version, string(raw))
	return err
}

func (s *Store) GetRelease(ctx context.Context, id string) (domain.ProductRelease, error) {
	return scanRelease(s.db.QueryRowContext(ctx, `SELECT `+releaseColumns+` FROM product_releases WHERE id = ?`, id))
}

func (s *Store) ListReleases(ctx context.Context, productID string) ([]domain.ProductRelease, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+releaseColumns+` FROM product_releases WHERE product_id = ? AND status<>'deleted' ORDER BY created_at DESC, id DESC`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ProductRelease{}
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

func (s *Store) SetReleaseStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE product_releases SET status = ? WHERE id = ?`, status, id)
	return err
}

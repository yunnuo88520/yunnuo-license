package store

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

type ReleaseUsage struct {
	TotalBytes    int64 `json:"total_bytes"`
	TotalVersions int   `json:"total_versions"`
}

func (s *Store) ReleaseStorageUsage(ctx context.Context) (ReleaseUsage, error) {
	var u ReleaseUsage
	e := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size),0),COUNT(*) FROM product_releases WHERE status<>'deleted'`).Scan(&u.TotalBytes, &u.TotalVersions)
	return u, e
}
func (tx *Tx) LockReleaseStorage(ctx context.Context) error {
	_, e := tx.tx.ExecContext(ctx, `UPDATE release_storage_lock SET id=id WHERE id=1`)
	return e
}
func (tx *Tx) ReleaseCapacity(ctx context.Context, p domain.Product) (total, developer, product int64, versions int, err error) {
	err = tx.tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(size),0) FROM product_releases WHERE status<>'deleted'`).Scan(&total)
	if err != nil {
		return
	}
	err = tx.tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(size),0),COUNT(*) FROM product_releases WHERE status<>'deleted' AND product_id=?`, p.ID).Scan(&product, &versions)
	if err != nil {
		return
	}
	if p.DeveloperID != "" {
		err = tx.tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(size),0) FROM product_releases WHERE status<>'deleted' AND product_id IN (SELECT product_id FROM product_developers WHERE user_id=?)`, p.DeveloperID).Scan(&developer)
	}
	return
}
func (tx *Tx) CreateRelease(ctx context.Context, r domain.ProductRelease) error {
	_, e := tx.tx.ExecContext(ctx, `INSERT INTO product_releases (`+releaseWriteColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.ProductID, domain.ReleaseStorageVersion(r.Version, r.Target), r.Notes, r.Status, r.Filename, r.Size, r.SHA256, r.KeyVersion, r.ManifestToken, ts(r.CreatedAt))
	if e != nil {
		return e
	}
	return tx.saveReleaseTarget(ctx, r)
}
func (tx *Tx) ReleaseVersionExists(ctx context.Context, product, version string) (bool, error) {
	var n int
	e := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_releases WHERE product_id=? AND version=?`, product, version).Scan(&n)
	return n > 0, e
}
func (tx *Tx) GetRelease(ctx context.Context, id string) (domain.ProductRelease, error) {
	return scanRelease(tx.tx.QueryRowContext(ctx, `SELECT `+releaseColumns+` FROM product_releases WHERE id=?`, id))
}
func (tx *Tx) SetReleaseStatus(ctx context.Context, id, status string) error {
	_, e := tx.tx.ExecContext(ctx, `UPDATE product_releases SET status=? WHERE id=?`, status, id)
	return e
}

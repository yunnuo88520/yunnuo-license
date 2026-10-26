package service

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/filelock"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"os"
	"path/filepath"
	"time"
)

func (s *Service) DeleteRelease(ctx context.Context, product, id, actor string) error {
	if s.releaseDir == "" {
		return ErrInvalidRequest
	}
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	unlock, err := filelock.Acquire(lockCtx, s.releaseDir+".lock", false)
	if err != nil {
		return err
	}
	defer unlock()
	path, trash := filepath.Join(s.releaseDir, id), filepath.Join(s.releaseDir, ".trash-"+id)
	moved := false
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if e := tx.LockReleaseStorage(ctx); e != nil {
			return e
		}
		if e := tx.LockProduct(ctx, product); e != nil {
			return e
		}
		r, e := tx.GetRelease(ctx, id)
		if e != nil || r.ProductID != product || r.Status == "deleted" {
			return ErrReleaseNotFound
		}
		if r.Status == "published" {
			return AppError{Code: "RELEASE_PUBLISHED", Message: "请先下架版本，再删除升级包文件", HTTPStatus: 409}
		}
		if e = os.Rename(path, trash); e == nil {
			moved = true
		} else if !os.IsNotExist(e) {
			return e
		}
		if e = tx.SetReleaseStatus(ctx, id, "deleted"); e != nil {
			return e
		}
		return s.transactionAudit(ctx, tx, "admin", actor, product, "", "product.release.delete", map[string]any{"release_id": id, "sha256": r.SHA256, "size": r.Size})
	})
	if moved {
		r, e := s.currentStore().GetRelease(ctx, id)
		if e == nil && r.Status == "deleted" {
			os.Remove(trash)
		} else if e == nil {
			os.Rename(trash, path)
		}
	}
	return err
}

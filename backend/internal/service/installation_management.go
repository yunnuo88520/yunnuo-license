package service

import (
	"context"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"strings"
)

func (s *Service) LicenseInstallations(ctx context.Context, product, no, user string) ([]domain.Installation, error) {
	l, e := s.currentStore().GetLicenseByNo(ctx, no)
	if e != nil || product != "" && l.ProductID != product || user != "" && l.OwnerUserID != user {
		return nil, ErrLicenseNotFound
	}
	return s.currentStore().Installations(ctx, l.ID)
}
func (s *Service) DisableInstallation(ctx context.Context, product, no, id, user, actor, reason string) error {
	if strings.TrimSpace(reason) == "" || len(reason) > 500 {
		return ErrInvalidRequest
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		l, e := tx.GetLicenseByNo(ctx, no)
		if e != nil || product != "" && l.ProductID != product || user != "" && l.OwnerUserID != user {
			return ErrLicenseNotFound
		}
		if e = tx.LockLicense(ctx, l.ID); e != nil {
			return e
		}
		l, e = tx.GetLicenseByID(ctx, l.ID)
		if e != nil {
			return e
		}
		if user != "" && l.OwnerUserID != user {
			return ErrLicenseNotFound
		}
		if user != "" {
			p, e := tx.GetProduct(ctx, l.ProductID)
			if e != nil {
				return e
			}
			p = effectiveProduct(p, l.AuthorizationTerms)
			if e = s.guardBindingChange(ctx, tx, p, l); e != nil {
				return e
			}
			if e = tx.AddBindingChange(ctx, yncrypto.NewID("change"), l.ID, "", yncrypto.NewID("req"), s.instanceHash("instance-id", id), "instance", s.now()); e != nil {
				return e
			}
		}
		if e = tx.DisableInstallation(ctx, l.ID, id); e != nil {
			return e
		}
		kind := "admin"
		if user != "" {
			kind = "customer"
		}
		return s.transactionAudit(ctx, tx, kind, actor, l.ProductID, l.ID, "installation.disable", map[string]string{"instance_id": id, "reason": reason})
	})
}

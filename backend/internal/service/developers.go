package service

import (
	"context"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func (s *Service) ListDeveloperProducts(ctx context.Context, userID string) ([]domain.Product, error) {
	return s.currentStore().ListDeveloperProducts(ctx, userID)
}

// The developer identity always comes from the authenticated session, never the request body.
func (s *Service) RequireDeveloperProduct(ctx context.Context, productID, userID string) error {
	p, err := s.currentStore().GetProduct(ctx, productID)
	if err != nil {
		if store.IsNotFound(err) {
			return ErrProductNotFound
		}
		return err
	}
	if userID == "" || p.DeveloperID != userID {
		return ErrProductNotFound
	}
	return nil
}

func (s *Service) AssignDeveloper(ctx context.Context, productID, userID, actorID string) error {
	if _, err := s.ProductUpdates(ctx, productID); err != nil {
		return err
	}
	if userID != "" {
		user, err := s.currentStore().GetAdminUserByID(ctx, userID)
		if err != nil || user.Role != domain.AdminRoleDeveloper || user.Status != domain.AdminUserActive {
			return ErrInvalidRequest
		}
	}
	if err := s.currentStore().AssignDeveloper(ctx, productID, userID); err != nil {
		return err
	}
	_ = s.audit(ctx, "admin", actorID, productID, "", "", "product.developer.assign", "success", "")
	return nil
}

func (s *Service) SetDeveloperStatus(ctx context.Context, userID, status, actorID string) error {
	user, err := s.currentStore().GetAdminUserByID(ctx, userID)
	if err != nil || user.Role != domain.AdminRoleDeveloper {
		return ErrInvalidRequest
	}
	if status != domain.AdminUserActive && status != domain.AdminUserDisabled {
		return ErrInvalidRequest
	}
	if err := s.currentStore().SetDeveloperStatus(ctx, userID, status); err != nil {
		return err
	}
	_ = s.audit(ctx, "admin", actorID, "", "", "", "developer.status", "success", "")
	return nil
}

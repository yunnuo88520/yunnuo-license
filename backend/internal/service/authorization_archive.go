package service

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func (s *Service) AuthorizationArchive(ctx context.Context) ([]store.AuthorizationArchiveItem, error) {
	return s.currentStore().AuthorizationArchive(ctx)
}
func (s *Service) ValidateLicensingDeveloper(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	u, e := s.currentStore().GetAdminUserByID(ctx, id)
	if e != nil || u.Role != domain.AdminRoleDeveloper || u.Status != "active" {
		return ErrInvalidRequest
	}
	return nil
}
func (s *Service) LicensingDevelopers(ctx context.Context) ([]domain.AdminUser, error) {
	users, e := s.currentStore().ListAdminUsers(ctx)
	if e != nil {
		return nil, e
	}
	result := []domain.AdminUser{}
	for _, u := range users {
		if u.Role == domain.AdminRoleDeveloper && u.Status == "active" {
			result = append(result, u)
		}
	}
	return result, nil
}

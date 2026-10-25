package service

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func (s *Service) IntegrationProduct(ctx context.Context, id string) (domain.Product, error) {
	p, err := s.currentStore().GetProduct(ctx, id)
	if store.IsNotFound(err) {
		return domain.Product{}, ErrProductNotFound
	}
	return p, err
}

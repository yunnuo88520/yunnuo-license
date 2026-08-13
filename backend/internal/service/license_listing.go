package service

import (
	"context"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

type LicenseListFilter struct {
	Status    string
	ProductID string
	AgentID   string
	Query     string
	Page      int
	PageSize  int
}

type LicensePage struct {
	Items    []domain.License `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

func (s *Service) ListLicensesPage(ctx context.Context, filter LicenseListFilter) (LicensePage, error) {
	filter.Status = strings.TrimSpace(filter.Status)
	filter.ProductID = strings.TrimSpace(filter.ProductID)
	filter.AgentID = strings.TrimSpace(filter.AgentID)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Status != "" && filter.Status != domain.LicenseActive && filter.Status != domain.LicenseExpired && filter.Status != domain.LicenseRevoked {
		return LicensePage{}, ErrInvalidRequest
	}
	if len(filter.Query) > 100 {
		return LicensePage{}, ErrInvalidRequest
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	now := s.now()
	items, total, err := s.store.ListLicensesPage(ctx, filter.Status, filter.ProductID, filter.AgentID, filter.Query,
		filter.PageSize, (filter.Page-1)*filter.PageSize, now)
	if err != nil {
		return LicensePage{}, err
	}
	for i := range items {
		if items[i].Status == domain.LicenseActive && items[i].ExpiredAt != nil && !now.Before(*items[i].ExpiredAt) {
			items[i].Status = domain.LicenseExpired
		}
	}
	return LicensePage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

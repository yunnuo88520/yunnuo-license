package service

import (
	"context"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

const maxBrandAssetLength = 700 * 1024

type UpdateSiteSettingsInput struct {
	SiteName       string `json:"site_name"`
	BrowserTitle   string `json:"browser_title"`
	LogoDataURL    string `json:"logo_data_url"`
	FaviconDataURL string `json:"favicon_data_url"`
	ActorID        string `json:"-"`
}

func (s *Service) SiteSettings(ctx context.Context) (domain.SiteSettings, error) {
	return s.currentStore().GetSiteSettings(ctx)
}

func (s *Service) UpdateSiteSettings(ctx context.Context, input UpdateSiteSettingsInput) (domain.SiteSettings, error) {
	input.SiteName = strings.TrimSpace(input.SiteName)
	input.BrowserTitle = strings.TrimSpace(input.BrowserTitle)
	if input.SiteName == "" || input.BrowserTitle == "" || len([]rune(input.SiteName)) > 80 || len([]rune(input.BrowserTitle)) > 80 {
		return domain.SiteSettings{}, ErrInvalidRequest
	}
	if !validBrandAsset(input.LogoDataURL) || !validBrandAsset(input.FaviconDataURL) {
		return domain.SiteSettings{}, ErrInvalidRequest
	}
	settings := domain.SiteSettings{
		SiteName: input.SiteName, BrowserTitle: input.BrowserTitle,
		LogoDataURL: input.LogoDataURL, FaviconDataURL: input.FaviconDataURL, UpdatedAt: s.now(),
	}
	if err := s.currentStore().UpdateSiteSettings(ctx, settings); err != nil {
		return domain.SiteSettings{}, err
	}
	_ = s.audit(ctx, "admin", input.ActorID, "", "", "", "site_settings.update", "success", "")
	return settings, nil
}

func validBrandAsset(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > maxBrandAssetLength {
		return false
	}
	allowed := []string{
		"data:image/png;base64,",
		"data:image/jpeg;base64,",
		"data:image/webp;base64,",
		"data:image/x-icon;base64,",
		"data:image/vnd.microsoft.icon;base64,",
	}
	for _, prefix := range allowed {
		if strings.HasPrefix(value, prefix) && len(value) > len(prefix) {
			return true
		}
	}
	return false
}

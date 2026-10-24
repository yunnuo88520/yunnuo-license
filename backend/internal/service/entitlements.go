package service

import (
	"context"
	"errors"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

type LicensePolicy struct {
	LicenseMode        string `json:"license_mode"`
	UpdateDurationDays int    `json:"update_duration_days"`
	ExpireGraceDays    int    `json:"expire_grace_days"`
	Locked             bool   `json:"locked"`
}

func validLicensePolicy(mode string, updateDays, graceDays int) bool {
	return (mode == domain.LicenseSubscription || mode == domain.LicensePerpetual) && updateDays >= 0 && updateDays <= 36500 && graceDays >= 0 && graceDays <= 365
}

func (s *Service) LicensePolicy(ctx context.Context, productID string) (LicensePolicy, error) {
	p, err := s.currentStore().GetProduct(ctx, productID)
	if err != nil {
		if store.IsNotFound(err) {
			return LicensePolicy{}, ErrProductNotFound
		}
		return LicensePolicy{}, err
	}
	locked, err := s.currentStore().LicensePolicyLocked(ctx, productID)
	return LicensePolicy{LicenseMode: p.LicenseMode, UpdateDurationDays: p.UpdateDurationDays, ExpireGraceDays: p.ExpireGraceDays, Locked: locked}, err
}

func (s *Service) SetLicensePolicy(ctx context.Context, productID string, input LicensePolicy, actorID string) (LicensePolicy, error) {
	if !validLicensePolicy(input.LicenseMode, input.UpdateDurationDays, input.ExpireGraceDays) {
		return LicensePolicy{}, ErrInvalidRequest
	}
	if _, err := s.LicensePolicy(ctx, productID); err != nil {
		return LicensePolicy{}, err
	}
	err := s.currentStore().SetLicensePolicy(ctx, productID, input.LicenseMode, input.UpdateDurationDays, input.ExpireGraceDays, s.now())
	if errors.Is(err, store.ErrLicensePolicyLocked) {
		return LicensePolicy{}, AppError{Code: "LICENSE_POLICY_LOCKED", Message: "issued cards or licenses already use this policy; create a new product to use different terms", HTTPStatus: 409}
	}
	if err != nil {
		return LicensePolicy{}, err
	}
	_ = s.audit(ctx, "admin", actorID, productID, "", "", "product.license_policy", "success", "")
	return s.LicensePolicy(ctx, productID)
}

func usageGraceUntil(product domain.Product, lic domain.License) *time.Time {
	if lic.ExpiredAt == nil {
		return nil
	}
	until := lic.ExpiredAt.AddDate(0, 0, product.ExpireGraceDays)
	return &until
}

func beforeExpiry(now time.Time, until *time.Time) bool { return until == nil || now.Before(*until) }

func licenseCanUpdate(lic domain.License, now time.Time) bool {
	return lic.Status == domain.LicenseActive && beforeExpiry(now, lic.ExpiredAt) && beforeExpiry(now, lic.UpdatesExpiredAt)
}

func extendExpiry(expiry *time.Time, durationDays int, permanent bool, now time.Time) *time.Time {
	if expiry == nil || permanent {
		return nil
	}
	base := now
	if expiry.After(now) {
		base = *expiry
	}
	next := base.AddDate(0, 0, durationDays)
	return &next
}

package licensing

import (
	"context"
	"time"
)

type CustomerEntitlement struct {
	Application       PublicApplication    `json:"application"`
	License           License              `json:"license"`
	Activations       []CustomerActivation `json:"activations"`
	ApplicationStatus string               `json:"application_status"`
	UpdatesEnabled    bool                 `json:"updates_enabled"`
}
type CustomerActivation struct {
	ID             string     `json:"id"`
	Label          string     `json:"label"`
	BindingType    string     `json:"binding_type"`
	LastVerifiedAt time.Time  `json:"last_verified_at"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at"`
	DeactivatedAt  *time.Time `json:"deactivated_at"`
	Borrowed       bool       `json:"borrowed"`
}

func (s *Service) CustomerEntitlements(ctx context.Context) ([]CustomerEntitlement, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return nil, e
	}
	if p.Actor.Kind != "customer" {
		return nil, forbidden("ROLE_FORBIDDEN", "请使用客户账号查看自己的授权")
	}
	apps, e := s.registry.ListApplications(ctx, "")
	if e != nil {
		return nil, e
	}
	out := []CustomerEntitlement{}
	for _, app := range apps {
		scoped, _, repo, e := s.runtimeContext(ctx, app.ApplicationID, p.Actor)
		if e != nil {
			return nil, e
		}
		e = repo.View(scoped, func(v View) error {
			licenses, e := v.Licenses().Find(scoped, Filter{OwnerUserID: p.Actor.ID, ExcludeRenewals: true, Limit: 10000})
			if e != nil {
				return e
			}
			for _, l := range licenses {
				entry := CustomerEntitlement{Application: PublicApplication{app.ApplicationID, app.Name, app.AllowedBindings}, License: l, ApplicationStatus: app.Status, UpdatesEnabled: app.UpdatesEnabled, Activations: []CustomerActivation{}}
				activations, e := v.Activations().Find(scoped, Filter{LicenseID: l.ID, Limit: 10000})
				if e != nil {
					return e
				}
				for _, a := range activations {
					entry.Activations = append(entry.Activations, CustomerActivation{a.ID, a.Label, a.BindingType, a.LastVerifiedAt, a.LeaseExpiresAt, a.DeactivatedAt, a.Borrowed})
				}
				out = append(out, entry)
			}
			return nil
		})
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (s *Service) CustomerDeactivate(ctx context.Context, appID, licenseID, activationID, reason string) error {
	scoped, _, repo, e := s.licenseOwnerContext(ctx, appID, licenseID, true)
	if e != nil {
		return e
	}
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return e
	}
	if reason == "" {
		return invalid("请填写停用原因")
	}
	return repo.Update(scoped, func(tx Transaction) error {
		l, e := tx.Licenses().Get(scoped, licenseID)
		if e != nil || p.Actor.Kind == "customer" && l.OwnerUserID != p.Actor.ID {
			return notFound()
		}
		a, e := tx.Activations().Get(scoped, activationID)
		if e != nil || a.LicenseID != licenseID {
			return notFound()
		}
		now := s.now()
		if a.Borrowed && a.LeaseExpiresAt != nil && now.Before(*a.LeaseExpiresAt) {
			return conflict("BORROWED_SEAT_HELD", "离线借用席位需要等待借用到期后释放")
		}
		a.DeactivatedAt = &now
		a.DeactivationReason = "customer_disabled"
		a.UpdatedAt = now
		if e = tx.Activations().Save(scoped, a); e != nil {
			return e
		}
		return s.append(scoped, tx, "installation_deactivated", a.ID, map[string]any{"license_id": l.ID, "reason": reason})
	})
}

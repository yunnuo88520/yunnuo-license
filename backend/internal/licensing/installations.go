package licensing

import (
	"context"
	"strings"
	"time"
)

type PendingInput struct {
	AppID     string       `json:"app_id"`
	PublicKey string       `json:"public_key"`
	Binding   BindingInput `json:"binding"`
	Label     string       `json:"label"`
}

func (s *Service) RequestInstallation(ctx context.Context, in PendingInput, req SignedRequest) (PendingInstallation, error) {
	scoped, _, repo, e := s.authenticateInstallation(ctx, in.AppID, in.PublicKey, req)
	if e != nil {
		return PendingInstallation{}, e
	}
	hash, _ := PublicKeyFingerprint(in.PublicKey)
	var out PendingInstallation
	e = repo.Update(scoped, func(tx Transaction) error {
		app, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		if app.Status != "active" || !contains(app.AllowedBindings, in.Binding.Type) {
			return forbidden("APPLICATION_DISABLED", "应用或绑定方式未启用")
		}
		b, e := s.runtimeBinding(app, in.Binding, 128, req, hash)
		if e != nil {
			return e
		}
		old, e := tx.PendingInstallations().Find(scoped, Filter{PublicKeyHash: hash, BindingHash: b.Hash, Status: "pending", Limit: 1000})
		if e != nil {
			return e
		}
		for _, v := range old {
			if s.now().Before(v.ExpiresAt) {
				out = v
				return nil
			}
		}
		out = PendingInstallation{Record: s.record(in.AppID, "pending"), PublicKey: in.PublicKey, PublicKeyHash: hash, BindingType: b.Kind, BindingHash: b.Hash, ClientBindingHash: b.ClientHash, Display: b.Display, Status: "pending", ExpiresAt: s.now().Add(48 * time.Hour)}
		if e = tx.PendingInstallations().Insert(scoped, out); e != nil {
			return e
		}
		return s.append(scoped, tx, "installation_requested", out.ID, map[string]any{"binding_type": b.Kind, "masked_anchor": b.Display})
	})
	return out, e
}
func (s *Service) ListPendingInstallations(ctx context.Context, appID string) ([]PendingInstallation, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return nil, e
	}
	var out []PendingInstallation
	e = repo.View(scoped, func(v View) error {
		var e error
		out, e = v.PendingInstallations().Find(scoped, Filter{Limit: 1000})
		return e
	})
	return out, e
}
func (s *Service) ApproveInstallation(ctx context.Context, appID, pendingID, licenseID, status, reason string) error {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return e
	}
	if status != "approved" && status != "rejected" || strings.TrimSpace(reason) == "" {
		return invalid("请选择批准/拒绝并填写原因")
	}
	return repo.Update(scoped, func(tx Transaction) error {
		v, e := tx.PendingInstallations().Get(scoped, pendingID)
		if e != nil {
			return notFound()
		}
		if v.Status != "pending" || !s.now().Before(v.ExpiresAt) {
			return conflict("REQUEST_NOT_PENDING", "申请已处理或已过期")
		}
		if status == "approved" {
			l, e := tx.Licenses().Get(scoped, licenseID)
			if e != nil {
				return notFound()
			}
			app, e := tx.Application(scoped)
			if e != nil {
				return e
			}
			if e = checkUsable(app, l, s.now()); e != nil {
				return e
			}
			if !contains(l.Terms.BindingTypes, v.BindingType) {
				return invalid("许可证不支持申请的绑定方式")
			}
			v.LicenseID = licenseID
		}
		v.Status = status
		v.UpdatedAt = s.now()
		if e = tx.PendingInstallations().Save(scoped, v); e != nil {
			return e
		}
		return s.append(scoped, tx, "installation_"+status, pendingID, map[string]any{"license_id": licenseID, "reason": reason})
	})
}
func (s *Service) ClaimCustomerLicense(ctx context.Context, appID, cardCode string) (License, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return License{}, e
	}
	if p.Actor.Kind != "customer" {
		return License{}, forbidden("ROLE_FORBIDDEN", "仅客户可关联本人授权")
	}
	scoped, _, repo, e := s.runtimeContext(ctx, appID, p.Actor)
	if e != nil {
		return License{}, e
	}
	var l License
	e = repo.Update(scoped, func(tx Transaction) error {
		cards, e := tx.Cards().Find(scoped, Filter{KeyHash: s.cardHash(appID, cardCode), Limit: 1})
		if e != nil {
			return e
		}
		if len(cards) != 1 || cards[0].Purpose != "activate" || cards[0].Status == "revoked" {
			return forbidden("CARD_INVALID", "关联卡密无效")
		}
		card := cards[0]
		if card.RedeemUntil != nil && !s.now().Before(*card.RedeemUntil) {
			return forbidden("CARD_EXPIRED", "卡密已超过关联截止时间")
		}
		l, e = tx.Licenses().Get(scoped, card.BoundLicenseID)
		if e != nil {
			return e
		}
		if l.OwnerUserID != "" && l.OwnerUserID != p.Actor.ID {
			return conflict("LICENSE_ALREADY_OWNED", "许可证已关联其他客户，不能自行更换归属")
		}
		if l.OwnerUserID == p.Actor.ID {
			return nil
		}
		l.OwnerUserID = p.Actor.ID
		l.UpdatedAt = s.now()
		if e = tx.Licenses().Save(scoped, l); e != nil {
			return e
		}
		return s.append(scoped, tx, "license_owner_claimed", l.ID, map[string]any{"owner_user_id": p.Actor.ID})
	})
	return l, e
}
func (s *Service) AssignLicenseOwner(ctx context.Context, appID, licenseID, userID, reason string) error {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return e
	}
	if strings.TrimSpace(reason) == "" || userID == "" {
		return invalid("请选择客户并填写归属调整原因")
	}
	return repo.Update(scoped, func(tx Transaction) error {
		l, e := tx.Licenses().Get(scoped, licenseID)
		if e != nil {
			return notFound()
		}
		app, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		if l.OwnerUserID != "" && l.OwnerUserID != userID && !app.AllowOwnerTransfer {
			return forbidden("OWNER_TRANSFER_DISABLED", "此应用默认禁止变更已有授权归属，请先明确启用管理员转移权限")
		}
		if l.OwnerUserID != "" && l.OwnerUserID != userID {
			now := s.now()
			borrowed := true
			held, e := tx.Activations().Count(scoped, Filter{LicenseID: l.ID, ActiveAt: &now, Borrowed: &borrowed})
			if e != nil {
				return e
			}
			if held > 0 {
				return conflict("BORROWED_SEAT_HELD", "请等待离线借用席位到期后转移归属")
			}
			if e = tx.DeactivateAll(scoped, l.ID, now); e != nil {
				return e
			}
			if e = tx.InvalidateInvitations(scoped, l.ID, now); e != nil {
				return e
			}
		}
		previous := l.OwnerUserID
		l.OwnerUserID = userID
		l.UpdatedAt = s.now()
		if e = tx.Licenses().Save(scoped, l); e != nil {
			return e
		}
		return s.append(scoped, tx, "license_owner_assigned", l.ID, map[string]any{"from": previous, "to": userID, "reason": reason})
	})
}

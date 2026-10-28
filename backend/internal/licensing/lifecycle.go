package licensing

import (
	"context"
	"errors"
	"strings"
	"time"
)

func (s *Service) ListLicenses(ctx context.Context, appID string) ([]License, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return nil, e
	}
	var out []License
	e = repo.View(scoped, func(v View) error { var e error; out, e = v.Licenses().Find(scoped, Filter{Limit: 1000}); return e })
	return out, e
}
func (s *Service) ListActivations(ctx context.Context, appID, licenseID string) ([]Activation, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return nil, e
	}
	var out []Activation
	e = repo.View(scoped, func(v View) error {
		var e error
		out, e = v.Activations().Find(scoped, Filter{LicenseID: licenseID, Limit: 1000})
		return e
	})
	return out, e
}
func (s *Service) SetLicenseStatus(ctx context.Context, appID, licenseID, status, reason string) error {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return e
	}
	if strings.TrimSpace(reason) == "" || len(reason) > 500 {
		return invalid("请填写状态变更原因")
	}
	if status != "active" && status != "suspended" && status != "revoked" && status != "expired" {
		return invalid("授权状态无效")
	}
	return repo.Update(scoped, func(tx Transaction) error {
		l, e := tx.Licenses().Get(scoped, licenseID)
		if e != nil {
			return notFound()
		}
		if l.Status == "revoked" && status != "revoked" {
			return conflict("REVOCATION_FINAL", "吊销为最终状态，请重新签发许可证")
		}
		previous := l.Status
		l.Status = status
		l.Reason = reason
		l.UpdatedAt = s.now()
		if e = tx.Licenses().Save(scoped, l); e != nil {
			return e
		}
		return s.append(scoped, tx, "license_status_changed", licenseID, map[string]any{"from": previous, "to": status, "reason": reason})
	})
}

type InvitationResult struct {
	Invitation Invitation `json:"invitation"`
	Token      string     `json:"token"`
}

func (s *Service) CreateInvitation(ctx context.Context, appID, licenseID string, hours int) (InvitationResult, error) {
	scoped, _, repo, e := s.licenseOwnerContext(ctx, appID, licenseID, true)
	if e != nil {
		return InvitationResult{}, e
	}
	if hours < 1 || hours > 168 {
		return InvitationResult{}, invalid("安装邀请有效期为 1–168 小时")
	}
	raw, e := randomHex(24)
	if e != nil {
		return InvitationResult{}, e
	}
	var out InvitationResult
	e = repo.Update(scoped, func(tx Transaction) error {
		l, e := tx.Licenses().Get(scoped, licenseID)
		if e != nil {
			return notFound()
		}
		p, _ := PrincipalFrom(ctx)
		if p.Actor.Kind == "customer" && l.OwnerUserID != p.Actor.ID {
			return notFound()
		}
		app, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		if e = checkUsable(app, l, s.now()); e != nil {
			return e
		}
		out = InvitationResult{Invitation: Invitation{Record: s.record(appID, "invite"), LicenseID: l.ID, TokenHash: s.hash("invitation", appID, raw), Kind: "installation", OwnerUserID: l.OwnerUserID, ExpiresAt: s.now().Add(time.Duration(hours) * time.Hour)}, Token: raw}
		if e = tx.Invitations().Insert(scoped, out.Invitation); e != nil {
			return e
		}
		return s.append(scoped, tx, "installation_invited", licenseID, map[string]any{"invitation_id": out.Invitation.ID, "expires_at": out.Invitation.ExpiresAt})
	})
	return out, e
}

// licenseOwnerContext verifies ownership before exposing the application's
// repository. Customer operations additionally pin every requested license.
func (s *Service) licenseOwnerContext(ctx context.Context, appID, licenseID string, write bool) (context.Context, Application, Repository, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return ctx, Application{}, nil, e
	}
	if p.Actor.Kind != "customer" {
		return s.applicationContext(ctx, appID, write)
	}
	scoped, app, repo, e := s.runtimeContext(ctx, appID, p.Actor)
	if e != nil {
		return ctx, app, nil, e
	}
	var l License
	e = repo.View(scoped, func(v View) error { var e error; l, e = v.Licenses().Get(scoped, licenseID); return e })
	if e != nil || l.OwnerUserID != p.Actor.ID {
		return ctx, app, nil, notFound()
	}
	return scoped, app, repo, nil
}
func (s *Service) CustomerLicenses(ctx context.Context) ([]License, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return nil, e
	}
	if p.Actor.Kind != "customer" {
		return nil, forbidden("ROLE_FORBIDDEN", "仅客户可查询本人授权")
	}
	apps, e := s.registry.ListApplications(ctx, "")
	if e != nil {
		return nil, e
	}
	out := []License{}
	for _, app := range apps {
		scoped, _, repo, e := s.runtimeContext(ctx, app.ApplicationID, p.Actor)
		if e != nil {
			return nil, e
		}
		e = repo.View(scoped, func(v View) error {
			items, e := v.Licenses().Find(scoped, Filter{OwnerUserID: p.Actor.ID, ExcludeRenewals: true, Limit: 10000})
			out = append(out, items...)
			return e
		})
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (s *Service) RegisterBinding(ctx context.Context, appID, licenseID string, input BindingInput, reason string) error {
	scoped, app, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return e
	}
	if strings.TrimSpace(reason) == "" {
		return invalid("请填写预录绑定原因")
	}
	return repo.Update(scoped, func(tx Transaction) error {
		var e error
		app, e = tx.Application(scoped)
		if e != nil {
			return e
		}
		l, e := tx.Licenses().Get(scoped, licenseID)
		if e != nil {
			return notFound()
		}
		if !contains(l.Terms.BindingTypes, input.Type) {
			return invalid("套餐不支持此绑定方式")
		}
		b, e := s.normalizeBinding(appID, input, l.Terms.MaxDomainAliases)
		if e != nil {
			return e
		}
		rules, e := tx.Rules().Find(scoped, Filter{Limit: 10000})
		if e != nil {
			return e
		}
		if e = s.checkApplicationRules(app, rules, b, l.ID); e != nil {
			return e
		}
		if _, e = s.ensureAnchorGroup(scoped, tx, l, b); e != nil {
			return e
		}
		l.BindingType = b.Kind
		l.BindingHash = b.Hash
		l.UpdatedAt = s.now()
		if e = tx.Licenses().Save(scoped, l); e != nil {
			return e
		}
		return s.append(scoped, tx, "binding_registered", l.ID, map[string]any{"binding_type": b.Kind, "masked_anchor": b.Display, "reason": reason})
	})
}
func (s *Service) Rebind(ctx context.Context, appID, licenseID string, input BindingInput, reason string) error {
	scoped, app, repo, e := s.licenseOwnerContext(ctx, appID, licenseID, true)
	if e != nil {
		return e
	}
	p, _ := PrincipalFrom(ctx)
	if strings.TrimSpace(reason) == "" {
		return invalid("请填写换绑原因")
	}
	return repo.Update(scoped, func(tx Transaction) error {
		var e error
		app, e = tx.Application(scoped)
		if e != nil {
			return e
		}
		l, e := tx.Licenses().Get(scoped, licenseID)
		if e != nil {
			return notFound()
		}
		if p.Actor.Kind == "customer" && l.OwnerUserID != p.Actor.ID {
			return notFound()
		}
		now := s.now()
		if e = checkUsable(app, l, now); e != nil {
			return e
		}
		if !contains(l.Terms.BindingTypes, input.Type) {
			return invalid("套餐不支持此绑定方式")
		}
		if p.Actor.Kind == "customer" {
			if l.Terms.RebindLimit == 0 || l.Terms.RebindLimit > 0 && l.RebindsUsed >= l.Terms.RebindLimit {
				return forbidden("REBIND_LIMIT_EXCEEDED", "此授权不允许自助换绑或次数已用完")
			}
			if l.LastRebindAt != nil && now.Before(l.LastRebindAt.Add(time.Duration(l.Terms.RebindCooldownSeconds)*time.Second)) {
				return forbidden("REBIND_COOLDOWN", "尚处于换绑冷却期")
			}
		}
		b, e := s.normalizeBinding(appID, input, l.Terms.MaxDomainAliases)
		if e != nil {
			return e
		}
		rules, e := tx.Rules().Find(scoped, Filter{Limit: 10000})
		if e != nil {
			return e
		}
		if e = s.checkApplicationRules(app, rules, b, l.ID); e != nil {
			return e
		}
		borrowed := true
		held, e := tx.Activations().Count(scoped, Filter{LicenseID: l.ID, ActiveAt: &now, Borrowed: &borrowed})
		if e != nil {
			return e
		}
		if held > 0 {
			return conflict("BORROWED_SEAT_HELD", "授权仍持有离线席位，需等待借用到期后换绑")
		}
		if e = tx.RetireBindings(scoped, l.ID, now); e != nil {
			return e
		}
		if e = tx.DeactivateAll(scoped, l.ID, now); e != nil {
			return e
		}
		if e = tx.InvalidateInvitations(scoped, l.ID, now); e != nil {
			return e
		}

		if _, e = s.ensureAnchorGroup(scoped, tx, l, b); e != nil {
			return e
		}
		l.BindingType = b.Kind
		l.BindingHash = b.Hash
		l.RebindsUsed++
		l.LastRebindAt = &now
		l.UpdatedAt = now
		if e = tx.Licenses().Save(scoped, l); e != nil {
			return e
		}
		return s.append(scoped, tx, "binding_replaced", l.ID, map[string]any{"binding_type": b.Kind, "reason": reason, "all_previous_activations_deactivated": true})
	})
}
func (s *Service) DeactivateInstallation(ctx context.Context, appID, activationID, reason string) error {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return e
	}
	if strings.TrimSpace(reason) == "" {
		return invalid("请填写停用原因")
	}
	return repo.Update(scoped, func(tx Transaction) error {
		a, e := tx.Activations().Get(scoped, activationID)
		if e != nil {
			return notFound()
		}
		now := s.now()
		if a.Borrowed && a.LeaseExpiresAt != nil && now.Before(*a.LeaseExpiresAt) {
			return conflict("BORROWED_SEAT_HELD", "已签发的离线借用令牌仍有效，席位需等待借用到期")
		}
		a.DeactivatedAt = &now
		a.DeactivationReason = "administrative"
		a.UpdatedAt = now
		if e = tx.Activations().Save(scoped, a); e != nil {
			return e
		}
		return s.append(scoped, tx, "installation_deactivated", a.ID, map[string]any{"reason": reason})
	})
}
func (s *Service) redeemRenewal(ctx context.Context, tx Transaction, l *License, code string) error {
	if l.RenewalOnly {
		return forbidden("LICENSE_RENEWAL_ONLY", "续费凭证不能作为再次续费的目标")
	}
	app, err := tx.Application(ctx)
	if err != nil {
		return err
	}
	if app.Status != "active" {
		return forbidden("APPLICATION_DISABLED", "应用停用期间不能兑换续费卡")
	}
	if !contains(app.AllowedModels, l.Terms.Model) {
		return forbidden("MODEL_DISABLED", "应用已停用此授权模型")
	}

	if l.Status == "revoked" || l.Status == "suspended" {
		return forbidden("LICENSE_UNUSABLE", "暂停或吊销的授权不能使用续费卡恢复")
	}
	cards, e := tx.Cards().Find(ctx, Filter{KeyHash: s.cardHash(l.ApplicationID, code), Limit: 1})
	if e != nil {
		return e
	}
	if len(cards) != 1 {
		return forbidden("CARD_INVALID", "续费卡无效")
	}
	card := cards[0]
	if l.UpdatesBlocked && card.Purpose == "renew_updates" {
		return forbidden("UPDATES_BLOCKED", "更新受限期间不能兑换更新续费卡，请先申诉核实")
	}
	if card.Status == "used" && card.RedeemedLicenseID == l.ID {
		return nil
	}
	if card.Status != "unused" || card.Purpose == "activate" || card.RedeemUntil != nil && !s.now().Before(*card.RedeemUntil) {
		return forbidden("CARD_UNUSABLE", "卡密已使用、到期或不属于续费卡")
	}
	source, e := tx.Licenses().Get(ctx, card.BoundLicenseID)
	if e != nil {
		return e
	}
	if source.PlanID != l.PlanID && !contains(source.Terms.RenewalTargetPlanIDs, l.PlanID) {
		return forbidden("RENEWAL_PLAN_INCOMPATIBLE", "续费卡未被允许用于当前套餐，请购买对应的续费规格")
	}
	if source.Status != "active" {
		return forbidden("CARD_UNUSABLE", "续费凭证对应的销售授权已停用或吊销")
	}
	if source.ID == l.ID || source.Terms.Model != l.Terms.Model {
		return invalid("续费卡与目标授权模型不匹配")
	}
	now := s.now()
	switch card.Purpose {
	case "renew_usage":
		if l.Terms.Perpetual || source.Terms.Perpetual || source.Terms.DurationDays <= 0 {
			return invalid("永久授权不使用时长续费卡")
		}
		if l.ActivatedAt == nil {
			if l.Terms.DurationDays > 36500-source.Terms.DurationDays {
				return invalid("待激活授权的累计使用天数超过上限")
			}
			l.Terms.DurationDays += source.Terms.DurationDays
		} else {
			base := now
			if l.ExpiresAt != nil && l.ExpiresAt.After(base) {
				base = *l.ExpiresAt
			}
			v := base.AddDate(0, 0, source.Terms.DurationDays)
			l.ExpiresAt = &v
		}
		if l.Status == "expired" {
			l.Status = "active"
		}
	case "renew_updates":
		if source.Terms.UpdateDurationDays <= 0 {
			return invalid("此套餐不含更新时长")
		}
		if l.ActivatedAt == nil {
			if l.Terms.UpdateDurationDays > 36500-source.Terms.UpdateDurationDays {
				return invalid("待激活授权的累计更新天数超过上限")
			}
			l.Terms.UpdateDurationDays += source.Terms.UpdateDurationDays
		} else {
			base := now
			if l.UpdatesUntil != nil && l.UpdatesUntil.After(base) {
				base = *l.UpdatesUntil
			}
			v := base.AddDate(0, 0, source.Terms.UpdateDurationDays)
			l.UpdatesUntil = &v
		}
	default:
		return invalid("卡密用途无效")
	}
	card.Status = "used"
	card.RedeemedLicenseID = l.ID
	card.UsedAt = &now
	card.UpdatedAt = now
	source.Status = "revoked"
	source.Reason = "renewal_consumed"
	source.UpdatedAt = now
	l.UpdatedAt = now
	if e = tx.Cards().Save(ctx, card); e != nil {
		return e
	}
	if e = tx.Licenses().Save(ctx, source); e != nil {
		return e
	}
	if e = tx.Licenses().Save(ctx, *l); e != nil {
		return e
	}
	return s.append(ctx, tx, "license_renewed", l.ID, map[string]any{"purpose": card.Purpose, "source_license_id": source.ID, "card_id": card.ID})
}
func (s *Service) CustomerRenew(ctx context.Context, appID, licenseID, card string) (License, error) {
	scoped, _, repo, e := s.licenseOwnerContext(ctx, appID, licenseID, true)
	if e != nil {
		return License{}, e
	}
	var l License
	e = repo.Update(scoped, func(tx Transaction) error {
		var e error
		l, e = tx.Licenses().Get(scoped, licenseID)
		if e != nil {
			return notFound()
		}
		p, _ := PrincipalFrom(ctx)
		if p.Actor.Kind == "customer" && l.OwnerUserID != p.Actor.ID {
			return notFound()
		}
		return s.redeemRenewal(scoped, tx, &l, card)
	})
	return l, e
}

// AuditIntegrity verifies both event contents and the recorded head. It does
// not claim protection against an operator who controls the database AND the
// server's audit key; export checkpoints to an independent store if required.
func (s *Service) AuditIntegrity(ctx context.Context, appID string) (map[string]any, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return nil, e
	}
	var sequence int64
	previous := ""
	e = repo.View(scoped, func(v View) error {
		for {
			events, e := v.Audit().Find(scoped, Filter{Ascending: true, Offset: int(sequence), Limit: 1000})
			if e != nil {
				return e
			}
			if len(events) == 0 {
				break
			}
			for _, event := range events {
				if event.Sequence != sequence+1 || event.PreviousHash != previous {
					return errors.New("audit sequence or chain mismatch")
				}
				hash, e := s.auditSigner(event)
				if e != nil {
					return e
				}
				if !constantEqual(hash, event.EventHash) {
					return errors.New("audit event authentication failed")
				}
				sequence = event.Sequence
				previous = event.EventHash
			}
		}
		head, e := v.AuditHead(scoped)
		if e != nil {
			return e
		}
		if head.Sequence != sequence || head.Hash != previous {
			return errors.New("audit head mismatch")
		}
		return nil
	})
	return map[string]any{"valid": e == nil, "sequence": sequence, "head": previous}, e
}

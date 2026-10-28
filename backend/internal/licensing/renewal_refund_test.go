package licensing_test

import (
	"context"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"testing"
	"time"
)

func TestRefundRebuildsRenewalsAcrossExpiryGapsWithoutRemovingLaterPaidTime(t *testing.T) {
	for _, model := range []string{"subscription", "perpetual"} {
		t.Run(model, func(t *testing.T) {
			s, _, root, now := coreFixture(t)
			app := fullApplication(t, s, root)
			terms := core.Terms{Model: model, BindingTypes: []string{"installation"}, DurationDays: 1, UpdateDurationDays: 1}
			base := issue(t, s, root, app.AppID, terms, "")
			client := newInstall(t)
			binding := core.BindingInput{Type: "installation", Value: "renewal-refund-install"}
			a, e := activate(t, s, client, *now, core.ActivateInput{AppID: app.AppID, CardCode: base.Cards[0].Code, Binding: binding, Form: "gui"})
			if e != nil {
				t.Fatal(e)
			}
			purpose := "renew_usage"
			if model == "perpetual" {
				purpose = "renew_updates"
			}
			redeem := func(card string) {
				_, e := verify(t, s, client, *now, core.VerifyInput{AppID: app.AppID, ActivationID: a.ActivationID, Binding: binding, Form: "gui", Action: "renew", RenewalCard: card})
				if e != nil {
					t.Fatal(e)
				}
			}
			renewal := func() core.SaleResult {
				r, e := s.CreateSale(root, app.AppID, core.SaleInput{PlanID: base.Order.PlanID, Quantity: 1, DistributeCards: true, Purpose: purpose, Reason: "verified renewal receipt"}, true)
				if e != nil {
					t.Fatal(e)
				}
				return r
			}
			get := func() core.License {
				all, e := s.ListLicenses(root, app.AppID)
				if e != nil {
					t.Fatal(e)
				}
				for _, l := range all {
					if l.ID == base.Cards[0].LicenseID {
						return l
					}
				}
				t.Fatal("missing target")
				return core.License{}
			}
			until := func(l core.License) *time.Time {
				if model == "perpetual" {
					return l.UpdatesUntil
				}
				return l.ExpiresAt
			}
			original := *until(get())
			*now = original.Add(time.Hour)
			first := renewal()
			redeem(first.Cards[0].Code)
			firstExpiry := *until(get())
			*now = firstExpiry.Add(time.Hour)
			second := renewal()
			redeem(second.Cards[0].Code)
			paidExpiry := *until(get())
			if _, e = s.RefundSale(root, app.AppID, first.Order.ID, "first renewal refunded after later paid gap"); e != nil {
				t.Fatal(e)
			}
			if !until(get()).Equal(paidExpiry) {
				t.Fatal("refund erased later separately purchased time", until(get()), paidExpiry)
			}
			if _, e = s.RefundSale(root, app.AppID, first.Order.ID, "duplicate confirmation"); e != nil || !until(get()).Equal(paidExpiry) {
				t.Fatal("duplicate refund changed entitlements", e)
			}
			if _, e = s.RefundSale(root, app.AppID, second.Order.ID, "second renewal refunded"); e != nil {
				t.Fatal(e)
			}
			if !until(get()).Equal(original) {
				t.Fatal("refunded credits remained active", until(get()), original)
			}
			_, e = verify(t, s, client, *now, core.VerifyInput{AppID: app.AppID, ActivationID: a.ActivationID, Binding: binding, Form: "gui"})
			if model == "perpetual" {
				if e != nil {
					t.Fatal("update refund revoked perpetual usage", e)
				}
			} else {
				assertCode(t, e, "LICENSE_EXPIRED")
			}
			if _, e = s.AuditIntegrity(root, app.AppID); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestRefundBeforeActivationDoesNotStartClockOrAllowRenewalCredentialChaining(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	base := issue(t, s, root, app.AppID, core.Terms{Model: "subscription", BindingTypes: []string{"installation"}, DurationDays: 30, UpdateDurationDays: 30}, "")
	renewal := func() core.SaleResult {
		r, e := s.CreateSale(root, app.AppID, core.SaleInput{PlanID: base.Order.PlanID, Quantity: 1, DistributeCards: true, Purpose: "renew_usage", Reason: "manual renewal receipt"}, true)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	first, second := renewal(), renewal()
	if _, e := s.CustomerRenew(root, app.AppID, first.Cards[0].LicenseID, second.Cards[0].Code); e == nil {
		t.Fatal("renewal credential could be amplified by another card")
	}
	if _, e := s.CustomerRenew(root, app.AppID, base.Cards[0].LicenseID, first.Cards[0].Code); e != nil {
		t.Fatal(e)
	}
	if _, e := s.RefundSale(root, app.AppID, first.Order.ID, "preactivation refund"); e != nil {
		t.Fatal(e)
	}
	client := newInstall(t)
	a, e := activate(t, s, client, *now, core.ActivateInput{AppID: app.AppID, CardCode: base.Cards[0].Code, Binding: core.BindingInput{Type: "installation", Value: "activation-after-refund"}, Form: "gui"})
	if e != nil {
		t.Fatal(e)
	}
	expected := now.AddDate(0, 0, 30)
	if a.UsageExpiresAt == nil || !a.UsageExpiresAt.Equal(expected) {
		t.Fatal("preactivation refund changed paid base term", a.UsageExpiresAt, expected)
	}
	_, e = s.CustomerRenew(core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "customer", ID: "foreign"}}), app.AppID, base.Cards[0].LicenseID, second.Cards[0].Code)
	if e == nil {
		t.Fatal("foreign customer renewed target")
	}
}

package licensing_test

import (
	"context"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"testing"
	"time"
)

func TestRenewalScopesCannotUseCheapPlansOrExtendTheWrongEntitlement(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	plan, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Enterprise", Terms: core.Terms{Model: "subscription", BindingTypes: []string{"installation"}, DurationDays: 30, UpdateDurationDays: 10, MaxActivations: 2, Features: []string{"reports", "export"}}, Currency: "CNY"})
	if e != nil {
		t.Fatal(e)
	}
	sale := func(planID, purpose string) core.SaleResult {
		t.Helper()
		r, e := s.CreateSale(root, app.AppID, core.SaleInput{PlanID: planID, Quantity: 1, DistributeCards: true, Purpose: purpose, Reason: "paid acceptance"}, true)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	initial := sale(plan.ID, "activate")
	c := newInstall(t)
	a, e := activate(t, s, c, *now, core.ActivateInput{AppID: app.AppID, CardCode: initial.Cards[0].Code, Binding: core.BindingInput{Type: "installation", Value: "customer-install"}, Form: "gui"})
	if e != nil {
		t.Fatal(e)
	}
	customer := core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "customer", ID: "customer-A", Role: "customer"}})
	l, e := s.ClaimCustomerLicense(customer, app.AppID, initial.Cards[0].Code)
	if e != nil {
		t.Fatal(e)
	}
	usage, updates := *l.ExpiresAt, *l.UpdatesUntil
	renewUsage := sale(plan.ID, "renew_usage")
	if _, e = s.CreateInvitation(root, app.AppID, renewUsage.Cards[0].LicenseID, 24); e == nil {
		t.Fatal("renewal credential became an independently usable license")
	}
	l, e = s.CustomerRenew(customer, app.AppID, a.LicenseID, renewUsage.Cards[0].Code)
	if e != nil {
		t.Fatal(e)
	}
	if !l.ExpiresAt.Equal(usage.AddDate(0, 0, 30)) || !l.UpdatesUntil.Equal(updates) || l.Terms.MaxActivations != 2 || len(l.Terms.Features) != 2 {
		t.Fatal("renewal overwrote unrelated purchased terms", l)
	}
	replay, e := s.CustomerRenew(customer, app.AppID, a.LicenseID, renewUsage.Cards[0].Code)
	if e != nil || !replay.ExpiresAt.Equal(*l.ExpiresAt) {
		t.Fatal("retry applied renewal twice", replay, e)
	}
	renewUpdates := sale(plan.ID, "renew_updates")
	l, e = s.CustomerRenew(customer, app.AppID, a.LicenseID, renewUpdates.Cards[0].Code)
	if e != nil {
		t.Fatal(e)
	}
	if !l.UpdatesUntil.Equal(updates.AddDate(0, 0, 10)) || !l.ExpiresAt.Equal(*replay.ExpiresAt) {
		t.Fatal("update renewal changed use entitlement")
	}
	cheap, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Cheaper incompatible edition", Terms: core.Terms{Model: "subscription", BindingTypes: []string{"installation"}, DurationDays: 365, UpdateDurationDays: 365}, Currency: "CNY"})
	if e != nil {
		t.Fatal(e)
	}
	cheapCard := sale(cheap.ID, "renew_usage")
	_, e = s.CustomerRenew(customer, app.AppID, a.LicenseID, cheapCard.Cards[0].Code)
	assertCode(t, e, "RENEWAL_PLAN_INCOMPATIBLE")
	allowed, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Enterprise renewal", Terms: core.Terms{Model: "subscription", BindingTypes: []string{"installation"}, DurationDays: 15, UpdateDurationDays: 7, RenewalTargetPlanIDs: []string{plan.ID}}, Currency: "CNY"})
	if e != nil {
		t.Fatal(e)
	}
	allowedCard := sale(allowed.ID, "renew_usage")
	l, e = s.CustomerRenew(customer, app.AppID, a.LicenseID, allowedCard.Cards[0].Code)
	if e != nil || !l.ExpiresAt.Equal(replay.ExpiresAt.AddDate(0, 0, 15)) {
		t.Fatal("explicit compatible SKU rejected", l, e)
	}
	if _, e = s.AuditIntegrity(root, app.AppID); e != nil {
		t.Fatal(e)
	}
}
func TestRenewalBeforeFirstActivationKeepsTheActivationStartDate(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	plan, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Start on activation", Terms: core.Terms{Model: "subscription", BindingTypes: []string{"installation"}, DurationDays: 30, UpdateDurationDays: 10}, Currency: "CNY"})
	if e != nil {
		t.Fatal(e)
	}
	issueSame := func(purpose string) core.SaleResult {
		r, e := s.CreateSale(root, app.AppID, core.SaleInput{PlanID: plan.ID, Quantity: 1, DistributeCards: true, Purpose: purpose, Reason: "paid"}, true)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	base, renewal := issueSame("activate"), issueSame("renew_usage")
	customer := core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "customer", ID: "owner", Role: "customer"}})
	l, e := s.ClaimCustomerLicense(customer, app.AppID, base.Cards[0].Code)
	if e != nil {
		t.Fatal(e)
	}
	l, e = s.CustomerRenew(customer, app.AppID, l.ID, renewal.Cards[0].Code)
	if e != nil || l.ActivatedAt != nil || l.ExpiresAt != nil || l.Terms.DurationDays != 60 {
		t.Fatal(l, e)
	}
	*now = now.Add(5 * 24 * time.Hour)
	c := newInstall(t)
	got, e := activate(t, s, c, *now, core.ActivateInput{AppID: app.AppID, CardCode: base.Cards[0].Code, Binding: core.BindingInput{Type: "installation", Value: "stable"}, Form: "gui"})
	if e != nil || got.UsageExpiresAt == nil || !got.UsageExpiresAt.Equal(now.AddDate(0, 0, 60)) {
		t.Fatal("pre-activation renewal lost or started too early", got, e)
	}
}

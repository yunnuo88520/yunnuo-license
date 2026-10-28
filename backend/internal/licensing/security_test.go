package licensing_test

import (
	"context"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"strings"
	"testing"
)

func TestIntegrityReviewAndAppealNeverGrantOrLaunderUpdateRights(t *testing.T) {
	s, _, root, now := coreFixture(t)
	s.WithReleaseStorage(t.TempDir(), core.StorageLimits{})
	app := fullApplication(t, s, root)
	enableUpdates(t, s, root, app)
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "perpetual", BindingTypes: []string{"installation"}, UpdateDurationDays: 30}, "")
	licenseID := sale.Cards[0].LicenseID
	if e := s.AssignLicenseOwner(root, app.AppID, licenseID, "customer-one", "verified original owner"); e != nil {
		t.Fatal(e)
	}
	client := newInstall(t)
	binding := core.BindingInput{Type: "installation", Value: "customer-install"}
	activated, e := activate(t, s, client, *now, core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, Binding: binding, Form: "web"})
	if e != nil {
		t.Fatal(e)
	}
	report := core.IntegrityReportInput{AppID: app.AppID, ActivationID: activated.ActivationID, Signal: "binary_modified", EvidenceSHA256: strings.Repeat("a", 64), Summary: "Controlled product integrity evidence"}
	impostor := newInstall(t)
	if _, e = s.ReportIntegrity(context.Background(), report, impostor.request(t, *now, "/v2/security/report", app.AppID, report)); e == nil {
		t.Fatal("someone else's installation reported as authenticated")
	}
	receipt, e := s.ReportIntegrity(context.Background(), report, client.request(t, *now, "/v2/security/report", app.AppID, report))
	if e != nil || receipt["status"] != "pending" {
		t.Fatal(receipt, e)
	}
	if e = s.ReviewLicenseSecurity(root, app.AppID, licenseID, core.SecurityDecisionInput{Action: "deny_updates", Reason: "manual evidence reviewed", ReferenceEventID: receipt["event_id"]}); e != nil {
		t.Fatal(e)
	}
	if _, e = verify(t, s, client, *now, core.VerifyInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: binding, Form: "web"}); e != nil {
		t.Fatal("update restriction took away usage", e)
	}
	update := core.UpdateInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: binding, Form: "web", CurrentVersion: "1.0.0", Target: core.ReleaseTarget{OS: "any", Arch: "any", Runtime: "any", Channel: "stable", Kind: "archive"}}
	_, e = s.CheckUpdate(context.Background(), update, client.request(t, *now, "/v2/updates/check", app.AppID, update))
	assertCode(t, e, "UPDATES_BLOCKED")
	renewal, e := s.CreateSale(root, app.AppID, core.SaleInput{PlanID: sale.Order.PlanID, Quantity: 1, DistributeCards: true, Purpose: "renew_updates", Reason: "paid renewal"}, true)
	if e != nil {
		t.Fatal(e)
	}
	_, e = verify(t, s, client, *now, core.VerifyInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: binding, Form: "web", Action: "renew", RenewalCard: renewal.Cards[0].Code})
	assertCode(t, e, "UPDATES_BLOCKED")
	cards, e := s.ListCards(root, app.AppID)
	if e != nil {
		t.Fatal(e)
	}
	for _, card := range cards {
		if card.ID == renewal.Cards[0].CardID && card.Status != "unused" {
			t.Fatal("restricted renewal consumed a card")
		}
	}
	customer := func(id string) context.Context {
		return core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "customer", ID: id}})
	}
	if _, e = s.SubmitSecurityAppeal(customer("other"), app.AppID, licenseID, "Please review my purchase"); e == nil {
		t.Fatal("foreign customer appeal accepted")
	}
	appeal, e := s.SubmitSecurityAppeal(customer("customer-one"), app.AppID, licenseID, "Please review my purchase")
	if e != nil || appeal["status"] != "pending" {
		t.Fatal(appeal, e)
	}
	if e = s.ReviewLicenseSecurity(root, app.AppID, licenseID, core.SecurityDecisionInput{Action: "allow_updates", Reason: "appeal verified", ReferenceEventID: appeal["event_id"]}); e != nil {
		t.Fatal(e)
	}
	if _, e = verify(t, s, client, *now, core.VerifyInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: binding, Form: "web", Action: "renew", RenewalCard: renewal.Cards[0].Code}); e != nil {
		t.Fatal("reviewed appeal did not permit paid renewal", e)
	}
	if _, e = s.AuditIntegrity(root, app.AppID); e != nil {
		t.Fatal(e)
	}
}

package licensing_test

import (
	"context"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"testing"
	"time"
)

func TestApprovalRebindingCooldownOwnershipAndInvitationInvalidation(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "perpetual", BindingTypes: []string{"domain"}, RebindLimit: 2, RebindCooldownSeconds: 60}, "")
	lid := sale.Cards[0].LicenseID
	owner := core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "customer", ID: "owner-a"}})
	stranger := core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "customer", ID: "owner-b"}})
	if _, e := s.ClaimCustomerLicense(owner, app.AppID, sale.Cards[0].Code); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ClaimCustomerLicense(stranger, app.AppID, sale.Cards[0].Code); e == nil {
		t.Fatal("license stolen by another customer")
	}
	binding := core.BindingInput{Type: "domain", Value: "old.example.com"}
	if e := s.RegisterBinding(root, app.AppID, lid, binding, "contract deployment"); e != nil {
		t.Fatal(e)
	}
	client := newInstall(t)
	pending := core.PendingInput{AppID: app.AppID, PublicKey: client.pub, Binding: binding}
	requested, e := s.RequestInstallation(context.Background(), pending, client.request(t, *now, "/v2/installations/request", app.AppID, pending))
	if e != nil {
		t.Fatal(e)
	}
	input := core.ActivateInput{AppID: app.AppID, PendingID: requested.ID, Binding: binding, Form: "web"}
	if _, e = activate(t, s, client, *now, input); e == nil {
		t.Fatal("unapproved public request granted usage")
	}
	if e = s.ApproveInstallation(root, app.AppID, requested.ID, lid, "approved", "customer purchase verified"); e != nil {
		t.Fatal(e)
	}
	if _, e = activate(t, s, newInstall(t), *now, input); e == nil {
		t.Fatal("approved installation stolen by another private key")
	}
	first, e := activate(t, s, client, *now, input)
	if e != nil {
		t.Fatal(e)
	}
	oldInvite, e := s.CreateInvitation(owner, app.AppID, lid, 24)
	if e != nil {
		t.Fatal(e)
	}
	replacement := core.BindingInput{Type: "domain", Value: "new.example.com"}
	if e = s.Rebind(stranger, app.AppID, lid, replacement, "not my contract"); e == nil {
		t.Fatal("foreign customer rebound license")
	}
	if e = s.Rebind(owner, app.AppID, lid, replacement, "customer moves website"); e != nil {
		t.Fatal(e)
	}
	_, e = verify(t, s, client, *now, core.VerifyInput{AppID: app.AppID, ActivationID: first.ActivationID, Binding: binding, Form: "web"})
	assertCode(t, e, "ACTIVATION_DEACTIVATED")
	if _, e = activate(t, s, newInstall(t), *now, core.ActivateInput{AppID: app.AppID, InvitationID: oldInvite.Invitation.ID, InvitationToken: oldInvite.Token, Binding: replacement, Form: "web"}); e == nil {
		t.Fatal("pre-rebinding invitation survived")
	}
	fresh, e := s.CreateInvitation(owner, app.AppID, lid, 24)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = activate(t, s, newInstall(t), *now, core.ActivateInput{AppID: app.AppID, InvitationID: fresh.Invitation.ID, InvitationToken: fresh.Token, Binding: replacement, Form: "container"}); e != nil {
		t.Fatal("new installation did not recover", e)
	}
	third := core.BindingInput{Type: "domain", Value: "third.example.com"}
	e = s.Rebind(owner, app.AppID, lid, third, "immediate move")
	assertCode(t, e, "REBIND_COOLDOWN")
	*now = now.Add(61 * time.Second)
	if e = s.Rebind(owner, app.AppID, lid, third, "approved second move"); e != nil {
		t.Fatal(e)
	}
	*now = now.Add(61 * time.Second)
	e = s.Rebind(owner, app.AppID, lid, core.BindingInput{Type: "domain", Value: "fourth.example.com"}, "too many")
	assertCode(t, e, "REBIND_LIMIT_EXCEEDED")
	e = s.AssignLicenseOwner(root, app.AppID, lid, "owner-b", "transfer disabled by default")
	assertCode(t, e, "OWNER_TRANSFER_DISABLED")
	transfer := true
	if _, e = s.SaveApplication(root, app.AppID, core.CreateApplicationInput{Name: app.Application.Name, AllowedModels: app.Application.AllowedModels, AllowedBindings: app.Application.AllowedBindings, Forms: app.Application.Forms, VerifyPolicy: app.Application.VerifyPolicy, AllowOwnerTransfer: &transfer}, "active"); e != nil {
		t.Fatal(e)
	}
	before, e := s.CreateInvitation(owner, app.AppID, lid, 24)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.AssignLicenseOwner(root, app.AppID, lid, "owner-b", "verified transfer contract"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CreateInvitation(owner, app.AppID, lid, 24); e == nil {
		t.Fatal("former owner retained access")
	}
	if _, e = activate(t, s, newInstall(t), *now, core.ActivateInput{AppID: app.AppID, InvitationID: before.Invitation.ID, InvitationToken: before.Token, Binding: third, Form: "web"}); e == nil {
		t.Fatal("former owner's invitation survived transfer")
	}
	if _, e = s.CreateInvitation(stranger, app.AppID, lid, 24); e != nil {
		t.Fatal("new owner denied", e)
	}
	if _, e = s.AuditIntegrity(root, app.AppID); e != nil {
		t.Fatal(e)
	}
}

func TestLiveWildcardScopeDepthApexAndEmptyAllowlist(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	policy := app.Application.VerifyPolicy
	policy.DomainRuleMode = "application_allowlist"
	if _, e := s.SaveApplication(root, app.AppID, core.CreateApplicationInput{Name: app.Application.Name, AllowedModels: app.Application.AllowedModels, AllowedBindings: app.Application.AllowedBindings, Forms: app.Application.Forms, VerifyPolicy: policy}, "active"); e != nil {
		t.Fatal(e)
	}
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "perpetual", BindingTypes: []string{"domain"}}, "")
	client := newInstall(t)
	binding := core.BindingInput{Type: "domain", Value: "a.example.com"}
	input := core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, Binding: binding, Form: "web"}
	_, e := activate(t, s, client, *now, input)
	assertCode(t, e, "BINDING_NOT_ALLOWLISTED")
	rule, e := s.SaveBindingRule(root, app.AppID, "", core.BindingRule{Kind: "domain", Pattern: "*.example.com", Enabled: true, LicenseID: sale.Cards[0].LicenseID, SubdomainDepth: "one"})
	if e != nil {
		t.Fatal(e)
	}
	activated, e := activate(t, s, client, *now, input)
	if e != nil {
		t.Fatal(e)
	}
	verifyHost := func(host string, want bool) {
		t.Helper()
		_, e := verify(t, s, client, *now, core.VerifyInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: core.BindingInput{Type: "domain", Value: host}, Form: "web"})
		if (e == nil) != want {
			t.Fatalf("%s allowed=%v: %v", host, want, e)
		}
	}
	verifyHost("b.example.com", true)
	verifyHost("example.com", false)
	verifyHost("a.b.example.com", false)
	verifyHost("evil-example.com", false)
	rule.IncludeApex = true
	rule.SubdomainDepth = "all"
	if _, e = s.SaveBindingRule(root, app.AppID, rule.ID, rule); e != nil {
		t.Fatal(e)
	}
	verifyHost("example.com", true)
	verifyHost("a.b.example.com", true)
	if e = s.RemoveBindingRule(root, app.AppID, rule.ID, "contract whitelist removed"); e != nil {
		t.Fatal(e)
	}
	verifyHost("a.example.com", false)
	if _, e = s.SaveBindingRule(root, app.AppID, "", core.BindingRule{Kind: "domain", Pattern: "*.com", Enabled: true}); e == nil {
		t.Fatal("public suffix wildcard accepted")
	}
}

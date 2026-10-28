package licensing_test

import (
	"context"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"strings"
	"testing"
	"time"
)

func TestEveryModelAndBindingAcrossProductFormsAndStrictDeadlines(t *testing.T) {
	s, _, root, now := coreFixture(t)
	s.WithReleaseStorage(t.TempDir(), core.StorageLimits{})
	app := fullApplication(t, s, root)
	enableUpdates(t, s, root, app)
	target := core.ReleaseTarget{OS: "linux", Arch: "amd64", Runtime: "generic", Channel: "stable", Kind: "archive"}
	release := publish(t, s, root, app.AppID, "2.0.0", target)
	start := *now
	anchors := []core.BindingInput{
		{Type: "machine_id", Value: strings.Repeat("a", 64)},
		{Type: "domain", Value: "HTTPS://EXAMPLE.COM:443"},
		{Type: "domain", Value: "例子.中国"},
		{Type: "ip", Value: "203.0.113.8"},
		{Type: "ip", Value: "::ffff:203.0.113.8"},
		{Type: "installation", Value: "persistent-installation"},
		{Type: "account", Value: "customer-account-1"},
		{Type: "tenant", Value: "customer-tenant-1"},
		{Type: "cluster", Value: "persistent-cluster-1"},
	}
	// IPv6 uses the explicitly approved declaration policy because the request
	// helper's trusted egress is IPv4. Both values are reevaluated server-side.
	current := enableUpdates(t, s, root, app)
	current.VerifyPolicy.IPSource = "declared"
	if _, e := s.SaveApplication(root, app.AppID, core.CreateApplicationInput{Name: current.Name, AllowedModels: current.AllowedModels, AllowedBindings: current.AllowedBindings, Forms: current.Forms, VerifyPolicy: current.VerifyPolicy, UpdatesEnabled: &current.UpdatesEnabled}, "active"); e != nil {
		t.Fatal(e)
	}
	anchors = append(anchors, core.BindingInput{Type: "ip", Value: "2001:db8::8"})
	for _, model := range []string{"perpetual", "subscription", "metered", "floating"} {
		cases := anchors
		if model == "floating" {
			cases = []core.BindingInput{{Type: "seat"}}
		}
		for index, binding := range cases {
			t.Run(model+"/"+binding.Type+"/"+string(rune('a'+index)), func(t *testing.T) {
				*now = start
				terms := core.Terms{Model: model, BindingTypes: []string{binding.Type}, DurationDays: 1, UpdateDurationDays: 1, MaxSeats: 1, UsageQuota: 20, UsageUnit: "request", UsagePeriod: "lifetime"}
				sale := issue(t, s, root, app.AppID, terms, "")
				client := newInstall(t)
				activated, e := activate(t, s, client, *now, core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, Binding: binding, Form: "gui"})
				if e != nil {
					t.Fatal(e)
				}
				request := core.VerifyInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: binding, Form: "gui"}
				for _, form := range app.Application.Forms {
					request.Form = form
					if _, e = verify(t, s, client, *now, request); e != nil {
						t.Fatalf("same package %s: %v", form, e)
					}
				}
				request.Form = "gui"
				if e = s.SetLicenseStatus(root, app.AppID, activated.LicenseID, "suspended", "matrix pause"); e != nil {
					t.Fatal(e)
				}
				_, e = verify(t, s, client, *now, request)
				assertCode(t, e, "LICENSE_SUSPENDED")
				if e = s.SetLicenseStatus(root, app.AppID, activated.LicenseID, "active", "matrix resume"); e != nil {
					t.Fatal(e)
				}
				if _, e = verify(t, s, client, *now, request); e != nil {
					t.Fatal(e)
				}
				update := core.UpdateInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: binding, Form: "gui", CurrentVersion: "1.0.0", Target: target}
				checks := func(wantSuccess bool) {
					t.Helper()
					result, e := s.CheckUpdate(context.Background(), update, client.request(t, *now, "/v2/updates/check", app.AppID, update))
					if wantSuccess {
						if e != nil || !result.Available {
							t.Fatalf("check before deadline: %v", e)
						}
					} else if e == nil {
						t.Fatal("expired check allowed")
					}
					_, file, e := s.DownloadUpdate(context.Background(), update, release.ID, client.request(t, *now, "/v2/updates/"+release.ID+"/download", app.AppID, update))
					if file != nil {
						file.Close()
					}
					if wantSuccess {
						if e != nil {
							t.Fatalf("download before deadline: %v", e)
						}
					} else if e == nil || file != nil {
						t.Fatal("expired download opened package")
					}
					_, e = s.AuthorizeUpdate(context.Background(), update, release.ID, client.request(t, *now, "/v2/updates/"+release.ID+"/authorize", app.AppID, update))
					if wantSuccess {
						if e != nil {
							t.Fatalf("install before deadline: %v", e)
						}
					} else if e == nil {
						t.Fatal("expired installation authorized")
					}
				}
				*now = start.Add(24*time.Hour - time.Second)
				if _, e = verify(t, s, client, *now, request); e != nil {
					t.Fatal("premature usage expiry", e)
				}
				checks(true)
				for _, offset := range []time.Duration{24 * time.Hour, 24*time.Hour + time.Second} {
					*now = start.Add(offset)
					checks(false)
					_, e = verify(t, s, client, *now, request)
					if model == "perpetual" {
						if e != nil {
							t.Fatal("update expiry removed perpetual use", e)
						}
					} else {
						assertCode(t, e, "LICENSE_EXPIRED")
					}
				}
				*now = start.Add(time.Minute)
				if e = s.SetLicenseStatus(root, app.AppID, activated.LicenseID, "revoked", "matrix final revoke"); e != nil {
					t.Fatal(e)
				}
				_, e = verify(t, s, client, *now, request)
				assertCode(t, e, "LICENSE_REVOKED")
				checks(false)
			})
		}
	}
	if _, e := s.AuditIntegrity(root, app.AppID); e != nil {
		t.Fatal(e)
	}
}

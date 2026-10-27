package service

import (
	"context"
	"fmt"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"strings"
	"sync"
	"testing"
	"time"
)

func scenarioFixture(t *testing.T, mode string, edit func(*domain.AuthorizationTerms)) (*Service, domain.Product, string, ActivateInput) {
	t.Helper()
	s := newTestService(t).WithReleaseDir(t.TempDir())
	ctx := context.Background()
	p, e := s.CreateProduct(ctx, CreateProductInput{Name: "Universal product", BindMode: "device"})
	if e != nil {
		t.Fatal(e)
	}
	terms := domain.LegacyTerms(p)
	terms.BindMode = mode
	terms.LicenseMode = "perpetual"
	terms.UpdateDurationDays = 7
	terms.RequireInstance = true
	terms.MaxDeployments = 2
	terms.MaxBindings = 1
	terms.UnbindLimit = 3
	terms.ExpireGraceDays = 0
	if edit != nil {
		edit(&terms)
	}
	if _, e = s.SetAuthorizationTerms(ctx, p.ID, "admin", terms); e != nil {
		t.Fatal(e)
	}
	card := createTestCard(t, ctx, s, p.ID, 30)
	return s, p, card, ActivateInput{AppKey: p.ID, CardCode: card, BindMode: mode, ClientInstance: domain.ClientInstance{InstanceID: "installation_0001", InstanceSecret: strings.Repeat("ab", 32), DeviceID: "physical-device-1"}}
}
func scenarioActivate(t *testing.T, s *Service, in ActivateInput) (LicenseResponse, VerifyInput) {
	t.Helper()
	r, e := s.Activate(context.Background(), in)
	if e != nil {
		t.Fatal(e)
	}
	return r, VerifyInput{ClientInstance: in.ClientInstance, AppKey: in.AppKey, LicenseNo: r.LicenseNo, BindMode: in.BindMode, BindValue: in.BindValue}
}

func TestScenarioWildcardScopeSnapshotAndPublicQuery(t *testing.T) {
	ctx := context.Background()
	s, p, card, in := scenarioFixture(t, "domain_wildcard", nil)
	in.BindValue = "*.EXAMPLE.com."
	stock := createTestCard(t, ctx, s, p.ID, 30)
	issued, verify := scenarioActivate(t, s, in)
	terms := *issued.AuthorizationTerms
	terms.BindMode = "ip"
	terms.MaxDeployments = 10
	if _, e := s.SetAuthorizationTerms(ctx, p.ID, "admin", terms); e != nil {
		t.Fatal(e)
	}
	for _, host := range []string{"a.example.com", "B.example.com."} {
		verify.BindValue = host
		if r, e := s.Verify(ctx, verify); e != nil || !r.CanUse {
			t.Fatalf("valid host %s: %+v %v", host, r, e)
		}
	}
	for _, host := range []string{"example.com", "a.b.example.com", "badexample.com", "example.com.evil.test", "*.example.com"} {
		verify.BindValue = host
		_, e := s.Verify(ctx, verify)
		assertCode(t, e, "BINDING_MISMATCH")
	}
	verify.BindValue = "a.example.com"
	decision, e := s.SecurityDecision(ctx, verify)
	if e != nil || !decision.CanUse {
		t.Fatal(decision, e)
	}
	result, e := s.QueryPublicLicense(ctx, PublicLicenseQueryInput{QueryType: "domain", Query: "a.example.com"})
	if e != nil || len(result.Results) != 1 || result.Results[0].ProductID != p.ID || strings.Contains(fmt.Sprint(result), issued.LicenseNo) {
		t.Fatal("wildcard public query missing or exposed credentials", e)
	}
	in.CardCode = stock
	in.InstanceID = "installation_0002"
	if r, e := s.Activate(ctx, in); e != nil || r.AuthorizationTerms.BindMode != "domain_wildcard" {
		t.Fatal("stock card terms changed", e)
	}
	in.CardCode = card
	in.InstanceID = "installation_0001"
	restored, e := s.Activate(ctx, in)
	if e != nil || restored.LicenseNo != issued.LicenseNo || !restored.UpdatesExpiredAt.Equal(*issued.UpdatesExpiredAt) {
		t.Fatal("reinstall changed grant", e)
	}
	_, e = s.UpdateProductProfile(ctx, p.ID, UpdateProductInput{Name: p.Name, BindMode: "domain", MaxBindCount: 1, OfflineGraceDays: 15}, "admin")
	assertCode(t, e, "BINDING_POLICY_LOCKED")
}

func TestScenarioWildcardApexDepthIDNAAndPublicSuffix(t *testing.T) {
	for _, pattern := range []string{"*.com", "*.co.uk", "*.github.io", "*", "bad*.example.com"} {
		if _, e := normalizeExtendedBinding("domain_wildcard", pattern); e == nil {
			t.Fatalf("unsafe pattern accepted %s", pattern)
		}
	}
	pattern, e := normalizeExtendedBinding("domain_wildcard", "*.例子.中国")
	if e != nil || !strings.HasPrefix(pattern, "*.xn--") {
		t.Fatal(pattern, e)
	}
	terms := domain.AuthorizationTerms{IncludeApex: true, SubdomainDepth: "all"}
	for _, host := range []string{"example.com", "a.b.example.com"} {
		if !matchBinding("domain_wildcard", "*.example.com", host, terms) {
			t.Fatal(host)
		}
	}
}

func TestScenarioCompositeCIDRAndInstanceProof(t *testing.T) {
	ctx := context.Background()
	s, _, _, in := scenarioFixture(t, "composite", func(v *domain.AuthorizationTerms) { v.BindingKinds = []string{"domain_wildcard", "device"} })
	in.Bindings = map[string]string{"domain_wildcard": "*.example.com", "device": "device-A"}
	_, verify := scenarioActivate(t, s, in)
	verify.Bindings = map[string]string{"domain_wildcard": "app.example.com", "device": "device-A"}
	if _, e := s.Verify(ctx, verify); e != nil {
		t.Fatal(e)
	}
	verify.Bindings = map[string]string{"domain_wildcard": "app.example.com", "device": "device-B"}
	_, e := s.Verify(ctx, verify)
	assertCode(t, e, "BINDING_MISMATCH")
	verify.Bindings = map[string]string{"domain_wildcard": "app.example.com"}
	_, e = s.Verify(ctx, verify)
	assertCode(t, e, "BINDING_REQUIRED")
	for _, network := range []string{"192.0.2.0/24", "2001:db8::/48"} {
		t.Run(network, func(t *testing.T) {
			svc, _, _, input := scenarioFixture(t, "ip_cidr", nil)
			input.BindValue = network
			_, v := scenarioActivate(t, svc, input)
			v.BindValue = map[string]string{"192.0.2.0/24": "192.0.2.99", "2001:db8::/48": "2001:db8:0:1::1"}[network]
			if _, e := svc.Verify(ctx, v); e != nil {
				t.Fatal(e)
			}
			v.InstanceSecret = strings.Repeat("cd", 32)
			_, e := svc.Verify(ctx, v)
			assertCode(t, e, "INSTANCE_INVALID")
			v.InstanceSecret = input.InstanceSecret
			v.BindValue = "198.51.100.1"
			_, e = svc.Verify(ctx, v)
			assertCode(t, e, "BINDING_MISMATCH")
		})
	}
}

func TestScenarioDeploymentDeviceLimitsAndClaim(t *testing.T) {
	ctx := context.Background()
	s, p, _, in := scenarioFixture(t, "domain_wildcard", func(v *domain.AuthorizationTerms) { v.MaxDevices = 1 })
	in.BindValue = "*.example.com"
	issued, verify := scenarioActivate(t, s, in)
	verify.BindValue = "a.example.com"
	in.InstanceID = "installation_0002"
	if _, e := s.Activate(ctx, in); e != nil {
		t.Fatal(e)
	}
	in.InstanceID = "installation_0003"
	_, e := s.Activate(ctx, in)
	assertCode(t, e, "DEPLOYMENT_LIMIT_EXCEEDED")
	ticket, e := s.CreateLicenseClaim(ctx, verify)
	if e != nil {
		t.Fatal(e)
	}
	owner, e := s.CreateCustomer(ctx, CustomerInput{Username: "owner", Password: "Owner-password-123"}, "admin")
	if e != nil {
		t.Fatal(e)
	}
	other, e := s.CreateCustomer(ctx, CustomerInput{Username: "other", Password: "Other-password-123"}, "admin")
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if _, e = s.RedeemLicenseClaim(ctx, owner.ID, ticket["claim_token"].(string)); e != nil {
			t.Fatal(e)
		}
	}
	_, e = s.RedeemLicenseClaim(ctx, other.ID, ticket["claim_token"].(string))
	assertCode(t, e, "OWNER_REQUIRED")
	if e = s.DisableInstallation(ctx, p.ID, issued.LicenseNo, "installation_0002", "", "admin", "replace damaged server"); e != nil {
		t.Fatal(e)
	}
	in.DeviceID = "different-device"
	_, e = s.Activate(ctx, in)
	assertCode(t, e, "DEVICE_LIMIT_EXCEEDED")
	in.DeviceID = "physical-device-1"
	if _, e = s.Activate(ctx, in); e != nil {
		t.Fatal(e)
	}
	verify.DeviceID = "spoofed-other-device"
	_, e = s.Verify(ctx, verify)
	assertCode(t, e, "BINDING_MISMATCH")
}

func TestScenarioSeatsAtomicAcquisitionBorrowAndExpiry(t *testing.T) {
	ctx := context.Background()
	s, _, _, in := scenarioFixture(t, "device", func(v *domain.AuthorizationTerms) { v.MaxConcurrentSeats = 1; v.OfflineGraceDays = 1 })
	in.BindValue = "workstation"
	_, verify := scenarioActivate(t, s, in)
	second := in
	second.InstanceID = "installation_0002"
	if _, e := s.Activate(ctx, second); e != nil {
		t.Fatal(e)
	}
	_, e := s.Verify(ctx, verify)
	assertCode(t, e, "SEAT_REQUIRED")
	var wg sync.WaitGroup
	results := make(chan domain.SeatLease, 12)
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			attempt := verify
			if i%2 == 1 {
				attempt.InstanceID = "installation_0002"
			}
			seat, e := s.AcquireSeat(ctx, SeatInput{VerifyInput: attempt, SessionID: fmt.Sprintf("session_%016d", i)})
			if e != nil {
				errors <- e
			} else {
				results <- seat
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(errors)
	winners := []domain.SeatLease{}
	for v := range results {
		winners = append(winners, v)
	}
	if len(winners) != 1 {
		t.Fatalf("oversold seats %d", len(winners))
	}
	for e := range errors {
		assertCode(t, e, "SEAT_LIMIT_EXCEEDED")
	}
	verify.InstanceID = winners[0].InstanceID
	verify.SessionID = winners[0].SessionID
	verify.SeatID = winners[0].ID
	if _, e = s.Verify(ctx, verify); e != nil {
		t.Fatal(e)
	}
	if e = s.ReleaseSeat(ctx, verify); e != nil {
		t.Fatal(e)
	}
	borrowed, e := s.AcquireSeat(ctx, SeatInput{VerifyInput: verify, SessionID: "borrowed_session_0001", BorrowDays: 1})
	if e != nil || !borrowed.Borrowed {
		t.Fatal(borrowed, e)
	}
	verify.SeatID = borrowed.ID
	if e = s.ReleaseSeat(ctx, verify); e != nil {
		t.Fatal(e)
	}
	_, e = s.AcquireSeat(ctx, SeatInput{VerifyInput: verify, SessionID: "another_session_0001"})
	assertCode(t, e, "SEAT_LIMIT_EXCEEDED")
	now := borrowed.ExpiresAt
	s.now = func() time.Time { return now }
	_, e = s.Verify(ctx, verify)
	assertCode(t, e, "SEAT_REQUIRED")
	if _, e = s.AcquireSeat(ctx, SeatInput{VerifyInput: verify, SessionID: "another_session_0001"}); e != nil {
		t.Fatal(e)
	}
}

func TestScenarioTargetedReleasesAndExactUpdateDeadline(t *testing.T) {
	ctx := context.Background()
	s, p, input := updateFixture(t)
	s.SetProductUpdates(ctx, p.ID, true, "admin")
	targets := []domain.ReleaseTarget{{OS: "linux", Arch: "amd64", Runtime: "go", Channel: "stable", Kind: "archive", MinVersion: "1.0.0", MaxVersion: "1.9.9"}, {OS: "windows", Arch: "amd64", Runtime: "go", Channel: "stable", Kind: "installer"}}
	releases := []domain.ProductRelease{}
	for _, target := range targets {
		r, e := s.UploadRelease(ctx, UploadReleaseInput{ProductID: p.ID, Version: "2.0.0", Filename: "product.zip", Target: &target}, strings.NewReader(target.OS), "admin")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.ChangeReleaseStatus(ctx, p.ID, r.ID, "published", "admin"); e != nil {
			t.Fatal(e)
		}
		releases = append(releases, r)
	}
	input.Target = &targets[0]
	input.CurrentVersion = "1.0.0"
	checked, e := s.CheckUpdate(ctx, CheckUpdateInput{VerifyInput: input, CurrentVersion: "1.0.0"})
	if e != nil || checked.Release == nil || checked.Release.ID != releases[0].ID {
		t.Fatal(checked, e)
	}
	_, e = s.AuthorizeRelease(ctx, releases[1].ID, input)
	assertCode(t, e, "RELEASE_NOT_FOUND")
	_, file, e := s.DownloadRelease(ctx, releases[0].ID, input)
	if e != nil {
		t.Fatal(e)
	}
	file.Close()
	var claims struct {
		Target  domain.ReleaseTarget `json:"target"`
		Version string               `json:"version"`
	}
	if e = yncrypto.VerifySignedJSON(p.PublicKeyPEM, releases[0].ManifestToken, &claims); e != nil || claims.Target.OS != "linux" || claims.Version != "2.0.0" {
		t.Fatal(claims, e)
	}
	input.CurrentVersion = "0.9.0"
	_, e = s.AuthorizeRelease(ctx, releases[0].ID, input)
	assertCode(t, e, "RELEASE_NOT_FOUND")
	l, e := s.currentStore().GetLicenseByNo(ctx, input.LicenseNo)
	if e != nil {
		t.Fatal(e)
	}
	s.now = func() time.Time { return *l.UpdatesExpiredAt }
	input.CurrentVersion = "1.0.0"
	_, e = s.AuthorizeRelease(ctx, releases[0].ID, input)
	assertCode(t, e, "LICENSE_EXPIRED")
}

// Exact deadlines apply identically to every binding shape. Usage grace cannot
// revive update downloads, and changing default rules cannot extend sold grants.
func TestScenarioEveryBindingDeadlineMatrix(t *testing.T) {
	ctx := context.Background()
	registrations := map[string]string{"domain": "app.example.com", "domain_wildcard": "*.example.com", "ip": "2001:db8::40", "ip_cidr": "192.0.2.0/24", "device": "device-A", "account": "buyer-A", "tenant": "Tenant_A", "instance": "installation_0001", "cluster": "cluster-A", "composite": `{"domain_wildcard":"*.example.com","device":"device-A"}`, "none": "none"}
	for mode, registration := range registrations {
		for _, licenseMode := range []string{"subscription", "perpetual"} {
			t.Run(mode+"/"+licenseMode, func(t *testing.T) {
				s, p, _, input := scenarioFixture(t, mode, func(v *domain.AuthorizationTerms) {
					v.LicenseMode = licenseMode
					v.ExpireGraceDays = 2
					if mode == "composite" {
						v.BindingKinds = []string{"domain_wildcard", "device"}
					}
				})
				input.BindValue = registration
				issued, verify := scenarioActivate(t, s, input)
				if mode == "domain_wildcard" {
					verify.BindValue = "app.example.com"
				}
				if mode == "ip_cidr" {
					verify.BindValue = "192.0.2.40"
				}
				if mode == "composite" {
					verify.BindValue = `{"domain_wildcard":"app.example.com","device":"device-A"}`
				}
				if _, e := s.SetProductUpdates(ctx, p.ID, true, "admin"); e != nil {
					t.Fatal(e)
				}
				release, e := s.UploadRelease(ctx, UploadReleaseInput{ProductID: p.ID, Version: "2.0.0", Filename: "product.zip"}, strings.NewReader("v2"), "admin")
				if e != nil {
					t.Fatal(e)
				}
				if _, e = s.ChangeReleaseStatus(ctx, p.ID, release.ID, "published", "admin"); e != nil {
					t.Fatal(e)
				}
				deadline := *issued.UpdatesExpiredAt
				s.now = func() time.Time { return deadline.Add(-time.Nanosecond) }
				if _, e = s.AuthorizeRelease(ctx, release.ID, verify); e != nil {
					t.Fatal("before update deadline", e)
				}
				s.now = func() time.Time { return deadline }
				if rights, e := s.Verify(ctx, verify); e != nil || !rights.CanUse || rights.CanUpdate {
					t.Fatal("update-only expiry", rights, e)
				}
				for _, step := range []string{"check", "authorize", "download"} {
					var err error
					switch step {
					case "check":
						_, err = s.CheckUpdate(ctx, CheckUpdateInput{VerifyInput: verify, CurrentVersion: "1.0.0"})
					case "authorize":
						_, err = s.AuthorizeRelease(ctx, release.ID, verify)
					case "download":
						_, file, e := s.DownloadRelease(ctx, release.ID, verify)
						err = e
						if file != nil {
							file.Close()
						}
					}
					assertCode(t, err, "UPDATES_EXPIRED")
				}
				if licenseMode == "subscription" {
					usage := *issued.UsageExpiredAt
					s.now = func() time.Time { return usage }
					if rights, e := s.Verify(ctx, verify); e != nil || !rights.CanUse || rights.CanUpdate {
						t.Fatal("usage grace must not extend updates", rights, e)
					}
					s.now = func() time.Time { return usage.AddDate(0, 0, 2) }
					_, e = s.Verify(ctx, verify)
					assertCode(t, e, "LICENSE_EXPIRED")
				} else {
					s.now = func() time.Time { return deadline.AddDate(2, 0, 0) }
					if rights, e := s.Verify(ctx, verify); e != nil || !rights.CanUse || rights.CanUpdate {
						t.Fatal("perpetual old version", rights, e)
					}
				}
			})
		}
	}
}

func TestScenarioIndependentRenewalCardsPreserveFrozenRights(t *testing.T) {
	ctx := context.Background()
	s, p, _, in := scenarioFixture(t, "device", func(v *domain.AuthorizationTerms) { v.LicenseMode = "subscription"; v.Features = []string{"export"} })
	in.BindValue = "device-A"
	issued, verify := scenarioActivate(t, s, in)
	usage, updates := *issued.UsageExpiredAt, *issued.UpdatesExpiredAt
	for _, purpose := range []string{"renew_updates", "renew_usage"} {
		terms := *issued.AuthorizationTerms
		terms.CardPurpose = purpose
		terms.UpdateDurationDays = 10
		terms.Features = []string{"unpaid-feature"}
		terms.MaxDeployments = 99
		plan, e := s.CreatePackage(ctx, domain.CardPackage{ProductID: p.ID, Name: purpose, DurationDays: 10, AuthorizationTerms: &terms}, "admin")
		if e != nil {
			t.Fatal(e)
		}
		batch, e := s.CreateCardBatch(ctx, CreateCardBatchInput{ProductID: p.ID, PackageID: plan.ID, Quantity: 1})
		if e != nil {
			t.Fatal(e)
		}
		invalid := in
		invalid.CardCode = batch.Codes[0]
		_, e = s.Activate(ctx, invalid)
		assertCode(t, e, "CARD_RENEWAL_ONLY")
		result, e := s.Renew(ctx, RenewInput{ClientInstance: verify.ClientInstance, AppKey: p.ID, LicenseNo: verify.LicenseNo, BindMode: verify.BindMode, BindValue: verify.BindValue, RenewCardCode: batch.Codes[0]})
		if e != nil {
			t.Fatal(e)
		}
		if purpose == "renew_updates" {
			updates = updates.AddDate(0, 0, 10)
		} else {
			usage = usage.AddDate(0, 0, 10)
		}
		if !result.UsageExpiredAt.Equal(usage) || !result.UpdatesExpiredAt.Equal(updates) || result.AuthorizationTerms.MaxDeployments != 2 || strings.Join(result.AuthorizationTerms.Features, ",") != "export" {
			t.Fatal("renewal changed unrelated rights", result)
		}
	}
}

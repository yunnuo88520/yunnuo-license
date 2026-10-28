package licensing_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"strings"
	"sync"
	"testing"
	"time"
)

type installClient struct {
	pub     string
	private ed25519.PrivateKey
}

func newInstall(t *testing.T) installClient {
	t.Helper()
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	return installClient{base64.RawURLEncoding.EncodeToString(pub), key}
}
func (c installClient) request(t *testing.T, now time.Time, path, app string, input any) core.SignedRequest {
	t.Helper()
	body, e := json.Marshal(input)
	if e != nil {
		t.Fatal(e)
	}
	nonce := make([]byte, 24)
	rand.Read(nonce)
	proof := core.RequestProof{Timestamp: now.Unix(), Nonce: base64.RawURLEncoding.EncodeToString(nonce)}
	proof.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(c.private, []byte(core.RequestSigningMessage("POST", path, app, proof.Timestamp, proof.Nonce, body))))
	return core.SignedRequest{Method: "POST", Path: path, RemoteIP: "203.0.113.8", Body: body, Proof: proof}
}
func fullApplication(t *testing.T, s *core.Service, ctx context.Context) core.CreatedApplication {
	t.Helper()
	a, e := s.CreateApplication(ctx, core.CreateApplicationInput{Name: "Unified product", AllowedModels: []string{"perpetual", "subscription", "floating", "metered"}, AllowedBindings: []string{"domain", "ip", "machine_id", "seat", "installation", "account", "tenant", "cluster"}, Forms: []string{"web", "container", "gui", "cli", "service", "cluster", "plugin", "mobile", "iot", "saas"}})
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func issue(t *testing.T, s *core.Service, ctx context.Context, app string, terms core.Terms, purpose string) core.SaleResult {
	t.Helper()
	p, e := s.SavePlan(ctx, app, "", core.PlanInput{Name: "Test plan", Terms: terms, Currency: "CNY"})
	if e != nil {
		t.Fatal(e)
	}
	sale, e := s.CreateSale(ctx, app, core.SaleInput{PlanID: p.ID, Quantity: 1, DistributeCards: true, Purpose: purpose, Reason: "test paid"}, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(sale.Cards) != 1 {
		t.Fatal("missing once-only card")
	}
	return sale
}
func activate(t *testing.T, s *core.Service, c installClient, now time.Time, in core.ActivateInput) (core.RuntimeResult, error) {
	t.Helper()
	in.PublicKey = c.pub
	return s.Activate(context.Background(), in, c.request(t, now, "/v2/activate", in.AppID, in))
}
func verify(t *testing.T, s *core.Service, c installClient, now time.Time, in core.VerifyInput) (core.RuntimeResult, error) {
	t.Helper()
	return s.Verify(context.Background(), in, c.request(t, now, "/v2/verify", in.AppID, in))
}
func assertCode(t *testing.T, e error, want string) {
	t.Helper()
	var ce core.Error
	var te *core.TokenError
	got := ""
	if errors.As(e, &ce) {
		got = ce.Code
	}
	if errors.As(e, &te) {
		got = te.Code
	}
	if got != want {
		t.Fatalf("wanted %s, got %v", want, e)
	}
}
func TestStandardPackageDomainAliasesExpiryAndRevocation(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "subscription", BindingTypes: []string{"domain"}, DurationDays: 1, UpdateDurationDays: 1, MaxActivations: 2, MaxDomainAliases: 2}, "")
	client := newInstall(t)
	input := core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, Binding: core.BindingInput{Type: "domain", Value: "HTTPS://EXAMPLE.COM:443", Aliases: []string{"example.com", "www.example.com"}}, Form: "web"}
	got, e := activate(t, s, client, *now, input)
	if e != nil {
		t.Fatal(e)
	}
	if len(strings.Split(got.JWT, ".")) != 3 {
		t.Fatal("not standard JWT")
	}
	input.Binding = core.BindingInput{Type: "domain", Value: "www.example.com"}
	input.Form = "container"
	second := newInstall(t)
	if _, e = activate(t, s, second, *now, input); e != nil {
		t.Fatal("same standard card must work across forms", e)
	}
	req := core.VerifyInput{AppID: app.AppID, ActivationID: got.ActivationID, JWT: got.JWT, Binding: input.Binding, Form: "web"}
	if _, e = verify(t, s, client, *now, req); e != nil {
		t.Fatal("registered alias denied", e)
	}
	req.Binding.Value = "evil.example.com"
	_, e = verify(t, s, client, *now, req)
	assertCode(t, e, "BINDING_MISMATCH")
	req.Binding.Value = "example.com"
	if e = s.SetLicenseStatus(root, app.AppID, got.LicenseID, "suspended", "test"); e != nil {
		t.Fatal(e)
	}
	_, e = verify(t, s, client, *now, req)
	assertCode(t, e, "LICENSE_SUSPENDED")
	if e = s.SetLicenseStatus(root, app.AppID, got.LicenseID, "active", "resume"); e != nil {
		t.Fatal(e)
	}
	*now = now.Add(24 * time.Hour)
	_, e = verify(t, s, client, *now, req)
	assertCode(t, e, "LICENSE_EXPIRED")
}
func TestInstallationSignatureReplayAndCrossApplication(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	other := fullApplication(t, s, root)
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "perpetual", BindingTypes: []string{"installation"}, UpdateDurationDays: 30}, "")
	c := newInstall(t)
	in := core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, PublicKey: c.pub, Binding: core.BindingInput{Type: "installation", Value: "persistent-id"}, Form: "gui"}
	signed := c.request(t, *now, "/v2/activate", app.AppID, in)
	got, e := s.Activate(context.Background(), in, signed)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Activate(context.Background(), in, signed)
	assertCode(t, e, "REQUEST_REPLAYED")
	req := core.VerifyInput{AppID: app.AppID, ActivationID: got.ActivationID, Binding: in.Binding, Form: "gui"}
	attacker := newInstall(t)
	_, e = verify(t, s, attacker, *now, req)
	assertCode(t, e, "REQUEST_SIGNATURE_INVALID")
	in.AppID = other.AppID
	_, e = activate(t, s, c, *now, in)
	assertCode(t, e, "CARD_INVALID")
	if e = s.SetLicenseStatus(root, app.AppID, got.LicenseID, "revoked", "stolen"); e != nil {
		t.Fatal(e)
	}
	_, e = verify(t, s, c, *now, req)
	assertCode(t, e, "LICENSE_REVOKED")
	if e = s.SetLicenseStatus(root, app.AppID, got.LicenseID, "active", "undo"); e == nil {
		t.Fatal("revocation must be final")
	}
	if report, e := s.AuditIntegrity(root, app.AppID); e != nil || report["valid"] != true {
		t.Fatal(report, e)
	}
}
func TestFloatingLeasesCrashExpiryBorrowAndRelease(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "floating", BindingTypes: []string{"seat"}, Perpetual: true, MaxSeats: 1}, "")
	c1, c2 := newInstall(t), newInstall(t)
	in := core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, Binding: core.BindingInput{Type: "seat"}, Form: "container"}
	one, e := activate(t, s, c1, *now, in)
	if e != nil {
		t.Fatal(e)
	}
	_, e = activate(t, s, c2, *now, in)
	assertCode(t, e, "SEAT_EXCEEDED")
	if one.GraceUntil.After(*one.LeaseExpiresAt) {
		t.Fatal("offline grant exceeds seat reservation")
	}
	*now = now.Add(121 * time.Second)
	two, e := activate(t, s, c2, *now, in)
	if e != nil {
		t.Fatal("crashed process must return seat after expiry", e)
	}
	req := core.VerifyInput{AppID: app.AppID, ActivationID: one.ActivationID, Binding: in.Binding, Form: "container"}
	_, e = verify(t, s, c1, *now, req)
	assertCode(t, e, "SEAT_EXCEEDED")
	req.ActivationID = two.ActivationID
	req.Action = "release"
	released, e := verify(t, s, c2, *now, req)
	if e != nil || !released.Released {
		t.Fatal(released, e)
	}
	in.BorrowSeconds = 600
	borrowed, e := activate(t, s, c1, *now, in)
	if e != nil {
		t.Fatal(e)
	}
	req.ActivationID = borrowed.ActivationID
	_, e = verify(t, s, c1, *now, req)
	assertCode(t, e, "BORROWED_SEAT_HELD")
	*now = now.Add(601 * time.Second)
	in.BorrowSeconds = 0
	if _, e = activate(t, s, c2, *now, in); e != nil {
		t.Fatal(e)
	}
}
func TestMeteringIdempotencyAndParallelQuota(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "metered", BindingTypes: []string{"installation"}, Perpetual: true, UsageQuota: 10, UsageUnit: "request", UsagePeriod: "monthly"}, "")
	c := newInstall(t)
	in := core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, Binding: core.BindingInput{Type: "installation", Value: "fixed"}, Form: "service"}
	got, e := activate(t, s, c, *now, in)
	if e != nil {
		t.Fatal(e)
	}
	req := core.VerifyInput{AppID: app.AppID, ActivationID: got.ActivationID, Binding: in.Binding, Form: "service", Action: "consume", Units: 4, IdempotencyKey: "invoice-1"}
	first, e := verify(t, s, c, *now, req)
	if e != nil || *first.RemainingUnits != 6 {
		t.Fatal(first, e)
	}
	again, e := verify(t, s, c, *now, req)
	if e != nil || *again.RemainingUnits != 6 {
		t.Fatal("duplicate charged", again, e)
	}
	req.Units = 3
	_, e = verify(t, s, c, *now, req)
	assertCode(t, e, "IDEMPOTENCY_CONFLICT")
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	failures := []error{}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r := req
			r.Units = 2
			r.IdempotencyKey = string(rune('a' + n))
			_, e := verify(t, s, c, *now, r)
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				success++
			} else {
				failures = append(failures, e)
			}
		}(i)
	}
	wg.Wait()
	if success != 3 {
		t.Fatal("quota oversold or unexpected transaction failure", success, failures)
	}
	*now = now.AddDate(0, 1, 0)
	req.Units = 4
	again, e = verify(t, s, c, *now, req)
	if e != nil || *again.RemainingUnits != 10 {
		t.Fatal("cross-period replay charged again", again, e)
	}
}
func TestPreRegisteredIPInvitationAndLiveCIDRRule(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "perpetual", BindingTypes: []string{"ip"}}, "")
	lid := sale.Cards[0].LicenseID
	rule, e := s.SaveBindingRule(root, app.AppID, "", core.BindingRule{Kind: "ip", Pattern: "203.0.113.0/24", LicenseID: lid, Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RegisterBinding(root, app.AppID, lid, core.BindingInput{Type: "ip", Value: "203.0.113.8"}, "customer IP"); e != nil {
		t.Fatal(e)
	}
	invite, e := s.CreateInvitation(root, app.AppID, lid, 24)
	if e != nil {
		t.Fatal(e)
	}
	c := newInstall(t)
	in := core.ActivateInput{AppID: app.AppID, InvitationID: invite.Invitation.ID, InvitationToken: invite.Token, Binding: core.BindingInput{Type: "ip", Value: "203.0.113.8"}, Form: "web"}
	got, e := activate(t, s, c, *now, in)
	if e != nil {
		t.Fatal(e)
	}
	req := core.VerifyInput{AppID: app.AppID, ActivationID: got.ActivationID, Binding: in.Binding, Form: "web"}
	req.Binding.Value = "203.0.113.9"
	signed := c.request(t, *now, "/v2/verify", app.AppID, req)
	signed.RemoteIP = "203.0.113.9"
	if _, e = s.Verify(context.Background(), req, signed); e != nil {
		t.Fatal("live CIDR denied", e)
	}
	if e = s.RemoveBindingRule(root, app.AppID, rule.ID, "remove access"); e != nil {
		t.Fatal(e)
	}
	_, e = verify(t, s, c, *now, req)
	if e == nil {
		t.Fatal("removed rule still allowed")
	}
	other := newInstall(t)
	_, e = activate(t, s, other, *now, in)
	assertCode(t, e, "INVITATION_INVALID")
}

func TestConcurrentFloatingCheckoutCannotOversellAndCrashReturnsCapacity(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "floating", BindingTypes: []string{"seat"}, Perpetual: true, MaxSeats: 3}, "")
	clients := make([]installClient, 12)
	for i := range clients {
		clients[i] = newInstall(t)
	}
	checkout := func() {
		t.Helper()
		var wg sync.WaitGroup
		results := make(chan error, len(clients))
		// Prepare signatures on the test goroutine; every caller receives its own
		// nonce, private key and immutable request at the same server clock instant.
		inputs := make([]core.ActivateInput, len(clients))
		proofs := make([]core.SignedRequest, len(clients))
		for i, c := range clients {
			inputs[i] = core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, PublicKey: c.pub, Binding: core.BindingInput{Type: "seat"}, Form: "container"}
			proofs[i] = c.request(t, *now, "/v2/activate", app.AppID, inputs[i])
		}
		for i := range clients {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, e := s.Activate(context.Background(), inputs[i], proofs[i])
				results <- e
			}(i)
		}
		wg.Wait()
		close(results)
		success := 0
		for e := range results {
			if e == nil {
				success++
			} else {
				assertCode(t, e, "SEAT_EXCEEDED")
			}
		}
		if success != 3 {
			t.Fatalf("concurrent checkout success=%d, max=3", success)
		}
	}
	checkout()
	*now = now.Add(121 * time.Second)
	checkout()
	if _, e := s.AuditIntegrity(root, app.AppID); e != nil {
		t.Fatal(e)
	}
}

package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func customerFixture(t *testing.T) (*Service, domain.Product, domain.Customer, domain.Customer, string, LicenseResponse) {
	t.Helper()
	ctx := context.Background()
	s := newTestService(t)
	a, err := s.CreateCustomer(ctx, CustomerInput{Username: "alice", Password: "alice-password", DisplayName: "Alice"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateCustomer(ctx, CustomerInput{Username: "bob", Password: "bob-password", DisplayName: "Bob"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProduct(ctx, CreateProductInput{Name: "Customer Ebox", Code: "CUST", BindMode: domain.BindDomain, LicenseMode: domain.LicensePerpetual})
	if err != nil {
		t.Fatal(err)
	}
	card := createTestCard(t, ctx, s, p.ID, 30)
	lic, err := s.CustomerActivate(ctx, a.ID, p.Code, ActivateInput{CardCode: card, BindValue: "old.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return s, p, a, b, card, lic
}

func TestCustomerOwnershipIsolationAndRenewal(t *testing.T) {
	ctx := context.Background()
	s, p, a, b, card, lic := customerFixture(t)
	owned, err := s.CustomerLicenses(ctx, a.ID)
	if err != nil || len(owned) != 1 || owned[0].License.OwnerUserID != a.ID {
		t.Fatalf("owner list: %+v %v", owned, err)
	}
	others, err := s.CustomerLicenses(ctx, b.ID)
	if err != nil || len(others) != 0 {
		t.Fatal("cross-user list exposed a license")
	}
	_, err = s.CustomerLicense(ctx, b.ID, lic.LicenseNo)
	assertCode(t, err, "LICENSE_NOT_FOUND")
	_, err = s.CustomerActivate(ctx, b.ID, p.Code, ActivateInput{CardCode: card, BindValue: "old.example.com"})
	assertCode(t, err, "OWNER_REQUIRED")
	renew := createTestCard(t, ctx, s, p.ID, 30)
	_, err = s.CustomerRenew(ctx, b.ID, lic.LicenseNo, owned[0].Bindings[0].ID, renew)
	assertCode(t, err, "LICENSE_NOT_FOUND")
	updated, err := s.CustomerRenew(ctx, a.ID, lic.LicenseNo, owned[0].Bindings[0].ID, renew)
	if err != nil || updated.UsageExpiredAt != nil || !updated.UpdatesExpiredAt.After(*lic.UpdatesExpiredAt) {
		t.Fatalf("owned renewal: %+v %v", updated, err)
	}
	_, err = s.CustomerRenew(ctx, a.ID, lic.LicenseNo, owned[0].Bindings[0].ID, renew)
	assertCode(t, err, "CARD_INVALID")
}

func TestCustomerRebindLimitsCooldownIdempotencyAndOldBindingInvalidation(t *testing.T) {
	ctx := context.Background()
	s, p, a, _, _, lic := customerFixture(t)
	view, err := s.CustomerLicense(ctx, a.ID, lic.LicenseNo)
	if err != nil {
		t.Fatal(err)
	}
	input := RebindInput{BindingID: view.Bindings[0].ID, NewValue: "new.example.com", RequestID: "first-request", Reason: "website migration"}
	_, err = s.CustomerRebind(ctx, a.ID, lic.LicenseNo, input)
	assertCode(t, err, "REBIND_DISABLED")
	if _, err = s.SetBindingPolicy(ctx, p.ID, "admin", domain.BindingPolicy{Allowed: true, Limit: 2, CooldownHours: 24}); err != nil {
		t.Fatal(err)
	}
	first, err := s.CustomerRebind(ctx, a.ID, lic.LicenseNo, input)
	if err != nil || first.ChangesUsed != 1 {
		t.Fatalf("first change: %+v %v", first, err)
	}
	retry, err := s.CustomerRebind(ctx, a.ID, lic.LicenseNo, input)
	if err != nil || retry.ChangesUsed != 1 {
		t.Fatal("retry consumed allowance twice")
	}
	_, err = s.Verify(ctx, VerifyInput{AppKey: p.AppKey, LicenseNo: lic.LicenseNo, BindMode: domain.BindDomain, BindValue: "old.example.com"})
	assertCode(t, err, "BINDING_MISMATCH")
	if _, err = s.Verify(ctx, VerifyInput{AppKey: p.AppKey, LicenseNo: lic.LicenseNo, BindMode: domain.BindDomain, BindValue: "new.example.com"}); err != nil {
		t.Fatal(err)
	}
	input.RequestID = "second-request"
	input.NewValue = "next.example.com"
	_, err = s.CustomerRebind(ctx, a.ID, lic.LicenseNo, input)
	assertCode(t, err, "UNBIND_COOLDOWN")
	now := s.now()
	s.now = func() time.Time { return now.Add(24 * time.Hour) }
	if second, err := s.CustomerRebind(ctx, a.ID, lic.LicenseNo, input); err != nil || second.ChangesUsed != 2 {
		t.Fatalf("cooldown boundary: %v", err)
	}
	input.RequestID = "third-request"
	input.NewValue = "third.example.com"
	_, err = s.CustomerRebind(ctx, a.ID, lic.LicenseNo, input)
	assertCode(t, err, "REBIND_LIMIT_REACHED")
	_, err = s.Unbind(ctx, UnbindInput{AppKey: p.AppKey, LicenseNo: lic.LicenseNo, BindMode: domain.BindDomain, BindValue: "next.example.com"})
	assertCode(t, err, "REBIND_LIMIT_REACHED")
}

func TestCustomerTransferRequiresAdministratorAndExplicitPolicy(t *testing.T) {
	ctx := context.Background()
	s, p, a, b, _, lic := customerFixture(t)
	input := AssignOwnerInput{UserID: b.ID, ExpectedOwner: a.ID, Reason: "approved corporate transfer"}
	err := s.AssignOwner(ctx, lic.LicenseNo, "admin", input)
	assertCode(t, err, "OWNER_TRANSFER_DISABLED")
	if _, err = s.SetBindingPolicy(ctx, p.ID, "admin", domain.BindingPolicy{AllowOwnerTransfer: true}); err != nil {
		t.Fatal(err)
	}
	input.ExpectedOwner = ""
	err = s.AssignOwner(ctx, lic.LicenseNo, "admin", input)
	assertCode(t, err, "OWNER_CHANGED")
	input.ExpectedOwner = a.ID
	if err = s.AssignOwner(ctx, lic.LicenseNo, "admin", input); err != nil {
		t.Fatal(err)
	}
	_, err = s.CustomerLicense(ctx, a.ID, lic.LicenseNo)
	assertCode(t, err, "LICENSE_NOT_FOUND")
	if _, err = s.CustomerLicense(ctx, b.ID, lic.LicenseNo); err != nil {
		t.Fatal(err)
	}
}

func TestCustomerSessionSeparationPasswordAndDisable(t *testing.T) {
	ctx := context.Background()
	s, _, a, _, _, _ := customerFixture(t)
	login, err := s.CustomerLogin(ctx, AdminLoginInput{Username: a.Username, Password: "alice-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateAdminToken(ctx, login.AccessToken); err == nil {
		t.Fatal("customer token became administrator")
	}
	if err = s.CustomerPassword(ctx, a.ID, ChangeAdminPasswordInput{CurrentPassword: "alice-password", NewPassword: "new-password-123"}); err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateCustomer(ctx, login.AccessToken)
	assertCode(t, err, "INVALID_USER_TOKEN")
	login, err = s.CustomerLogin(ctx, AdminLoginInput{Username: a.Username, Password: "new-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetCustomerStatus(ctx, a.ID, "disabled", "admin"); err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateCustomer(ctx, login.AccessToken)
	if err == nil {
		t.Fatal("disabled account session accepted")
	}
}

func TestConcurrentCustomerChangesCannotExceedAllowance(t *testing.T) {
	ctx := context.Background()
	s, p, a, _, _, lic := customerFixture(t)
	if _, err := s.SetBindingPolicy(ctx, p.ID, "admin", domain.BindingPolicy{Allowed: true, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	view, _ := s.CustomerLicense(ctx, a.ID, lic.LicenseNo)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, host := range []string{"one.example.com", "two.example.com"} {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			_, err := s.CustomerRebind(ctx, a.ID, lic.LicenseNo, RebindInput{BindingID: view.Bindings[0].ID, NewValue: host, RequestID: host, Reason: "concurrent migration"})
			results <- err
		}(host)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	view, err := s.CustomerLicense(ctx, a.ID, lic.LicenseNo)
	if err != nil || success != 1 || view.ChangesUsed != 1 {
		t.Fatalf("concurrent budget exceeded: success=%d used=%d err=%v", success, view.ChangesUsed, err)
	}
}

func TestCustomerRebindRollsBackWhenBusinessAuditFails(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "audit.db")
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	st, err := store.Open(ctx, "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	s := New(st, []byte("pepper"), []byte("0123456789abcdef0123456789abcdef"))
	defer s.Close()
	a, err := s.CreateCustomer(ctx, CustomerInput{Username: "alice", Password: "alice-password"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProduct(ctx, CreateProductInput{Name: "Audit", Code: "AUD", BindMode: domain.BindDomain})
	if err != nil {
		t.Fatal(err)
	}
	card := createTestCard(t, ctx, s, p.ID, 30)
	lic, err := s.CustomerActivate(ctx, a.ID, p.Code, ActivateInput{CardCode: card, BindValue: "old.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetBindingPolicy(ctx, p.ID, "admin", domain.BindingPolicy{Allowed: true, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	view, _ := s.CustomerLicense(ctx, a.ID, lic.LicenseNo)
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`CREATE TRIGGER fail_audit BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = s.CustomerRebind(ctx, a.ID, lic.LicenseNo, RebindInput{BindingID: view.Bindings[0].ID, NewValue: "new.example.com", RequestID: "audit-failure", Reason: "migration"})
	if err == nil {
		t.Fatal("audit failure allowed a binding mutation")
	}
	view, err = s.CustomerLicense(ctx, a.ID, lic.LicenseNo)
	if err != nil || view.ChangesUsed != 0 || view.Bindings[0].Value != "old.example.com" {
		t.Fatalf("failed audit left partial state: %+v %v", view, err)
	}
}

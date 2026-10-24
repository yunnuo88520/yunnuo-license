package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func securityFixture(t *testing.T, s *Service) (domain.Product, domain.Customer, VerifyInput) {
	t.Helper()
	ctx := context.Background()
	p, err := s.CreateProduct(ctx, CreateProductInput{Name: "Security", Code: "SEC", BindMode: domain.BindDomain})
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateCustomer(ctx, CustomerInput{Username: "customer", Password: "customer-password"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	card := createTestCard(t, ctx, s, p.ID, 30)
	lic, err := s.CustomerActivate(ctx, u.ID, p.Code, ActivateInput{CardCode: card, BindValue: "ebox.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return p, u, VerifyInput{AppKey: p.AppKey, LicenseNo: lic.LicenseNo, BindMode: domain.BindDomain, BindValue: "ebox.example.com"}
}
func TestSecuritySignalsRequireManualReviewAndDeduplicate(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p, _, in := securityFixture(t, s)
	wrong := in
	wrong.BindValue = "wrong.example.com"
	_, err := s.Verify(ctx, wrong)
	assertCode(t, err, "BINDING_MISMATCH")
	cases, _ := s.SecurityCases(ctx, "")
	if len(cases) != 0 {
		t.Fatal("disabled detection recorded a case")
	}
	if _, err = s.SetSecurityPolicy(ctx, p.ID, "admin", domain.SecurityPolicy{Enabled: true, DetectMismatch: true, AcceptReports: true, Action: "block_page", Message: "请核实授权"}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		_, err = s.Verify(ctx, wrong)
		assertCode(t, err, "BINDING_MISMATCH")
	}
	cases, err = s.SecurityCases(ctx, "")
	if err != nil || len(cases) != 1 || cases[0].Occurrences != 2 || cases[0].Status != "open" || cases[0].Action != "observe" {
		t.Fatal(cases, err)
	}
	if _, err = s.Verify(ctx, in); err != nil {
		t.Fatal("unconfirmed report disabled legitimate usage", err)
	}
	r := domain.ProductRelease{ID: "release-security", ProductID: p.ID, Version: "1.0.0", Status: "published", SHA256: strings.Repeat("a", 64), CreatedAt: s.now()}
	if err = s.currentStore().CreateRelease(ctx, r); err != nil {
		t.Fatal(err)
	}
	report, err := s.ReportIntegrity(ctx, IntegrityReportInput{VerifyInput: in, ReleaseID: r.ID, ObservedSHA256: strings.Repeat("b", 64)})
	if err != nil || report.Status != "open" {
		t.Fatal(report, err)
	}
	_, err = s.ReportIntegrity(ctx, IntegrityReportInput{VerifyInput: in, ReleaseID: r.ID, ObservedSHA256: r.SHA256})
	assertCode(t, err, "INTEGRITY_MATCHES")
	other := in
	other.AppKey = "other-key"
	if _, err = s.ReportIntegrity(ctx, IntegrityReportInput{VerifyInput: other, ReleaseID: r.ID, ObservedSHA256: strings.Repeat("c", 64)}); err == nil {
		t.Fatal("report crossed products")
	}
	d, err := s.SecurityDecision(ctx, in)
	if err != nil || !d.CanUse || !d.CanUpdate || d.Action != "observe" {
		t.Fatal(d, err)
	}
}
func TestSecurityActionsEnforceUsageUpdatesHeartbeatActivationAndRenewal(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p, u, in := securityFixture(t, s)
	if _, err := s.SetSecurityPolicy(ctx, p.ID, "admin", domain.SecurityPolicy{Enabled: true, DetectMismatch: true, Action: "block_page", Message: "请核实授权"}); err != nil {
		t.Fatal(err)
	}
	lic, _ := s.currentStore().GetLicenseByNo(ctx, in.LicenseNo)
	c, err := s.recordSecuritySignal(ctx, lic, "integrity_mismatch", "1.0.0", "untrusted client report")
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"observe", "deny_updates", "deny_use", "block_page"} {
		if err = s.ReviewSecurityCase(ctx, c.ID, "admin", ReviewSecurityInput{Status: "confirmed", Action: action, Reason: "人工核实"}); err != nil {
			t.Fatal(err)
		}
		d, err := s.SecurityDecision(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		wantUse := action == "observe" || action == "deny_updates"
		if d.CanUse != wantUse || d.CanUpdate != (action == "observe") {
			t.Fatalf("action %s: %+v", action, d)
		}
		v, verifyErr := s.Verify(ctx, in)
		_, heartErr := s.Heartbeat(ctx, in)
		_, updateErr := s.authorizeUpdate(ctx, in)
		view, err := s.CustomerLicense(ctx, u.ID, in.LicenseNo)
		if err != nil || view.CanUse != wantUse || view.CanUpdate != (action == "observe") {
			t.Fatal("user rights disagree", view, err)
		}
		if wantUse {
			if verifyErr != nil || heartErr != nil || v.CanUpdate != (action == "observe") {
				t.Fatal(action, v, verifyErr, heartErr)
			}
		} else {
			assertCode(t, verifyErr, "SECURITY_USE_DENIED")
			assertCode(t, heartErr, "SECURITY_USE_DENIED")
		}
		if action == "observe" {
			if updateErr != nil {
				t.Fatal(updateErr)
			}
		} else if action == "deny_updates" {
			assertCode(t, updateErr, "SECURITY_UPDATES_DENIED")
		} else {
			assertCode(t, updateErr, "SECURITY_USE_DENIED")
		}
	}
	card, err := s.currentStore().GetCard(ctx, lic.CardID)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := yncrypto.DecryptString(s.dataKey, card.CodeEncrypted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Activate(ctx, ActivateInput{AppKey: p.AppKey, CardCode: plain, BindMode: in.BindMode, BindValue: in.BindValue})
	assertCode(t, err, "SECURITY_USE_DENIED")
	renewCard := createTestCard(t, ctx, s, p.ID, 30)
	renew := RenewInput{AppKey: p.AppKey, LicenseNo: in.LicenseNo, BindMode: in.BindMode, BindValue: in.BindValue, RenewCardCode: renewCard}
	_, err = s.Renew(ctx, renew)
	assertCode(t, err, "SECURITY_USE_DENIED")
	policy, _ := s.SecurityPolicy(ctx, p.ID)
	policy.Enabled = false
	if _, err = s.SetSecurityPolicy(ctx, p.ID, "admin", policy); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(ctx, in); err != nil {
		t.Fatal("global application switch did not release enforcement", err)
	}
	policy.Enabled = true
	if _, err = s.SetSecurityPolicy(ctx, p.ID, "admin", policy); err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewSecurityCase(ctx, c.ID, "admin", ReviewSecurityInput{Status: "resolved", Reason: "排除误报"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Renew(ctx, renew); err != nil {
		t.Fatal("blocked renewal consumed the card or resolution failed", err)
	}
}
func TestSecurityAppealsEnforceOwnershipAndOrdinaryExpiryIsSeparate(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p, u, in := securityFixture(t, s)
	other, err := s.CreateCustomer(ctx, CustomerInput{Username: "other", Password: "other-password"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetSecurityPolicy(ctx, p.ID, "admin", domain.SecurityPolicy{Enabled: true, Action: "block_page", Message: "人工核实"}); err != nil {
		t.Fatal(err)
	}
	lic, _ := s.currentStore().GetLicenseByNo(ctx, in.LicenseNo)
	c, err := s.recordSecuritySignal(ctx, lic, "integrity_mismatch", "1.0.0", "client report")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AppealSecurityCase(ctx, other.ID, c.ID, "not my license")
	assertCode(t, err, "LICENSE_NOT_FOUND")
	a, err := s.AppealSecurityCase(ctx, u.ID, c.ID, "正版购买凭证")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.AppealSecurityCase(ctx, u.ID, c.ID, "重复提交")
	if err != nil || a.ID != again.ID {
		t.Fatal("duplicate appeal", err)
	}
	items, _ := s.SecurityCases(ctx, other.ID)
	if len(items) != 0 {
		t.Fatal("other user read a case")
	}
	items, _ = s.SecurityCases(ctx, u.ID)
	if len(items) != 1 || items[0].Evidence != "" {
		t.Fatal("private evidence exposed", items)
	}
	if err = s.ReviewSecurityCase(ctx, c.ID, "admin", ReviewSecurityInput{Status: "confirmed", Action: "block_page", Reason: "确认"}); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return lic.ActivatedAt.AddDate(0, 0, 31) }
	d, err := s.SecurityDecision(ctx, in)
	if err != nil || d.Action != "authorization_unavailable" || d.CaseID != "" || d.CanUse || d.CanUpdate {
		t.Fatal("expiry mislabeled as piracy", d, err)
	}
	if err = s.ReviewSecurityCase(ctx, c.ID, "admin", ReviewSecurityInput{Status: "resolved", Reason: "排除误报"}); err != nil {
		t.Fatal(err)
	}
	appeals, _ := s.SecurityAppeals(ctx, u.ID)
	if len(appeals) != 1 || appeals[0].Status != "closed" {
		t.Fatal(appeals)
	}
}
func TestSecurityReviewAuditFailureCannotEnforceRestriction(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "security.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	st, err := store.Open(ctx, "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	s := New(st, []byte("pepper"), []byte("0123456789abcdef0123456789abcdef"))
	defer s.Close()
	p, _, in := securityFixture(t, s)
	if _, err = s.SetSecurityPolicy(ctx, p.ID, "admin", domain.SecurityPolicy{Enabled: true, Action: "block_page", Message: "确认"}); err != nil {
		t.Fatal(err)
	}
	lic, _ := s.currentStore().GetLicenseByNo(ctx, in.LicenseNo)
	c, err := s.recordSecuritySignal(ctx, lic, "integrity_mismatch", "1.0.0", "client report")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`CREATE TRIGGER fail_security_review BEFORE INSERT ON audit_logs WHEN NEW.action='security.review' BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewSecurityCase(ctx, c.ID, "admin", ReviewSecurityInput{Status: "confirmed", Action: "block_page", Reason: "确认"}); err == nil {
		t.Fatal("audit failure still confirmed case")
	}
	d, err := s.SecurityDecision(ctx, in)
	if err != nil || !d.CanUse || d.Action != "observe" {
		t.Fatal("partial enforcement after audit failure", d, err)
	}
}

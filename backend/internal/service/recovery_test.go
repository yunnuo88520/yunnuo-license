package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecoveryExpirySingleUsePrivilegeBoundaryAndSessionInvalidation(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	root, _, err := s.EnsureBootstrapAdmin(ctx, "root", "root-password", "Root")
	if err != nil {
		t.Fatal(err)
	}
	customer, err := s.CreateCustomer(ctx, CustomerInput{Username: "recover-user", Password: "old-password"}, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.CustomerLogin(ctx, AdminLoginInput{Username: customer.Username, Password: "old-password"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateRecovery(ctx, "admin", root.ID, "regular-admin", domain.AdminRoleAdmin, RecoveryInput{Reason: "identity checked"})
	assertCode(t, err, "PERMISSION_DENIED")
	r, err := s.CreateRecovery(ctx, "customer", customer.ID, root.ID, domain.AdminRoleSuperAdmin, RecoveryInput{Reason: "identity checked", Minutes: 5})
	if err != nil {
		t.Fatal(err)
	}
	clock := s.now
	s.now = func() time.Time { return r.ExpiresAt }
	assertCode(t, s.RedeemRecovery(ctx, r.Code, "new-password"), "RECOVERY_INVALID")
	s.now = clock
	first, err := s.CreateRecovery(ctx, "customer", customer.ID, root.ID, domain.AdminRoleSuperAdmin, RecoveryInput{Reason: "reissue"})
	if err != nil {
		t.Fatal(err)
	}
	latest, err := s.CreateRecovery(ctx, "customer", customer.ID, root.ID, domain.AdminRoleSuperAdmin, RecoveryInput{Reason: "reissue again"})
	if err != nil {
		t.Fatal(err)
	}
	assertCode(t, s.RedeemRecovery(ctx, first.Code, "new-password"), "RECOVERY_INVALID")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.RedeemRecovery(ctx, latest.Code, "new-password") }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			assertCode(t, err, "RECOVERY_INVALID")
		}
	}
	if success != 1 {
		t.Fatal("recovery code used more than once", success)
	}
	if _, err = s.AuthenticateCustomer(ctx, old.AccessToken); err == nil {
		t.Fatal("old customer session survived reset")
	}
	if _, err = s.CustomerLogin(ctx, AdminLoginInput{Username: customer.Username, Password: "new-password"}); err != nil {
		t.Fatal(err)
	}
	stale, err := s.CreateRecovery(ctx, "customer", customer.ID, root.ID, domain.AdminRoleSuperAdmin, RecoveryInput{Reason: "before password change"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResetAccountPassword(ctx, "customer", customer.ID, "another-password", "operator reset", root.ID, domain.AdminRoleSuperAdmin); err != nil {
		t.Fatal(err)
	}
	assertCode(t, s.RedeemRecovery(ctx, stale.Code, "bad-password"), "RECOVERY_INVALID")
	audit, err := s.ListAuditLogs(ctx, AuditLogFilter{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(audit)
	for _, secret := range []string{latest.Code, first.Code, "new-password", "another-password"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("audit leaked recovery credential")
		}
	}
}
func TestAgentAndAdministratorPasswordsResetInvalidateExistingSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	root, _, err := s.EnsureBootstrapAdmin(ctx, "root", "root-password", "Root")
	if err != nil {
		t.Fatal(err)
	}
	oldRoot, err := s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "root-password"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateAgent(ctx, CreateAgentInput{Name: "Agent"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateAgentUser(ctx, CreateAgentUserInput{AgentID: a.ID, Username: "owner", Password: "old-password"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.AgentLogin(ctx, AgentLoginInput{AgentID: a.ID, Username: "owner", Password: "old-password"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResetAccountPassword(ctx, "agent", u.ID, "new-password", "verified", root.ID, domain.AdminRoleSuperAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateAgentToken(ctx, old.AccessToken); err == nil {
		t.Fatal("old agent token survived reset")
	}
	if _, err = s.AgentLogin(ctx, AgentLoginInput{AgentID: a.ID, Username: "owner", Password: "new-password"}); err != nil {
		t.Fatal(err)
	}
	if err = s.ResetAccountPassword(ctx, "admin", root.ID, "new-root-password", "verified", root.ID, domain.AdminRoleSuperAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateAdminToken(ctx, oldRoot.AccessToken); err == nil {
		t.Fatal("old administrator token survived reset")
	}
}
func TestRecoveryAuditFailureRollsBackPasswordAndCodeConsumption(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "recovery.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	st, err := store.Open(ctx, "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	s := New(st, []byte("pepper"), []byte("0123456789abcdef0123456789abcdef"))
	defer s.Close()
	u, err := s.CreateCustomer(ctx, CustomerInput{Username: "user", Password: "old-password"}, "root")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateRecovery(ctx, "customer", u.ID, "root", domain.AdminRoleSuperAdmin, RecoveryInput{Reason: "verified"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TRIGGER fail_recovery_audit BEFORE INSERT ON audit_logs WHEN NEW.action='account.recovery.redeem' BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.RedeemRecovery(ctx, r.Code, "new-password"); err == nil {
		t.Fatal("audit failure still reset password")
	}
	if _, err = s.CustomerLogin(ctx, AdminLoginInput{Username: u.Username, Password: "old-password"}); err != nil {
		t.Fatal("partial password reset", err)
	}
	if _, err = db.Exec(`DROP TRIGGER fail_recovery_audit`); err != nil {
		t.Fatal(err)
	}
	if err = s.RedeemRecovery(ctx, r.Code, "new-password"); err != nil {
		t.Fatal("failed transaction consumed recovery code", err)
	}
}

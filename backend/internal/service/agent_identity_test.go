package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func TestConcurrentOnboardingCannotDuplicateQualificationOrCredit(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	u, err := s.CreateCustomer(ctx, CustomerInput{Username: "concurrent-seller", Password: "secret123"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.OpenAgentAccount(ctx, OpenAgentInput{UserID: u.ID, InitialBalance: 1000, Reason: "Opening credit"}, "admin")
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			assertCode(t, err, "AGENT_ALREADY_GRANTED")
		}
	}
	agents, err := s.ListAgents(ctx)
	if err != nil || success != 1 || len(agents) != 1 {
		t.Fatalf("duplicate qualification: %d %+v %v", success, agents, err)
	}
	ledger, err := s.AgentMoneyLedgers(ctx, agents[0].ID)
	if err != nil || len(ledger) != 1 || ledger[0].Amount != 1000 {
		t.Fatal("duplicate opening credit", ledger, err)
	}
}

func TestOpenAgentAuditFailureRollsBackEveryBusinessRecord(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "atomic.db") + "?_pragma=foreign_keys(1)"
	st, err := store.Open(ctx, "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	s := New(st, []byte("pepper"), []byte("0123456789abcdef0123456789abcdef"))
	defer s.Close()
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	pkg, err := s.CreatePackage(ctx, domain.CardPackage{ProductID: p.ID, Name: "Monthly", DurationDays: 30, BaseCost: 500}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	_, err = raw.Exec(`CREATE TRIGGER fail_open BEFORE INSERT ON audit_logs WHEN NEW.action='agent.open' BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.OpenAgentAccount(ctx, OpenAgentInput{Username: "atomic", Password: "secret123", InitialBalance: 1000, Reason: "Opening", PackageIDs: []string{pkg.ID}}, "admin")
	if err == nil {
		t.Fatal("audit failure accepted")
	}
	for _, table := range []string{"customer_users", "agents", "agent_users", "agent_user_identities", "agent_accounts", "agent_package_grants", "agent_product_policies", "agent_money_ledgers"} {
		var count int
		if err = raw.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(table, err)
		}
		if count != 0 {
			t.Fatalf("partial onboarding retained %d rows in %s", count, table)
		}
	}
}

func TestLegacyAgentIdentityMigrationNeverMergesSameNamedPeople(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", "file:"+filepath.Join(t.TempDir(), "legacy.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	migrations := filepath.Join("..", "..", "migrations")
	if err = st.Migrate(ctx, migrations); err != nil {
		t.Fatal(err)
	}
	s := New(st, []byte("pepper"), []byte("0123456789abcdef0123456789abcdef"))
	defer s.Close()
	c, err := s.CreateCustomer(ctx, CustomerInput{Username: "owner", Password: "customer123"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateAgent(ctx, CreateAgentInput{Name: "Legacy Agency"})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := hashAgentPassword("agent1234")
	if err != nil {
		t.Fatal(err)
	}
	m := domain.AgentUser{ID: "aguser_legacy", AgentID: a.ID, Username: "owner", PasswordHash: hash, Role: domain.AgentRoleOwner, Status: domain.AgentUserActive, CreatedAt: s.now(), UpdatedAt: s.now()}
	if err = st.CreateAgentUser(ctx, m); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = st.Migrate(ctx, migrations); err != nil {
			t.Fatal(err)
		}
	}
	id, err := st.AgentIdentityID(ctx, m.ID)
	if err != nil || id == c.ID {
		t.Fatal("migration merged unrelated users", id, err)
	}
	identity, err := st.CustomerByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Username != "agent-legacy" {
		t.Fatal(identity.Username)
	}
	login, err := s.AgentLogin(ctx, AgentLoginInput{Username: identity.Username, Password: "agent1234"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateCustomer(ctx, login.AccessToken); err != nil {
		t.Fatal("migrated identity cannot use user center", err)
	}
	if _, err = s.CustomerLogin(ctx, AdminLoginInput{Username: "owner", Password: "customer123"}); err != nil {
		t.Fatal("original customer changed", err)
	}
	people, err := s.Customers(ctx)
	if err != nil || len(people) != 2 {
		t.Fatal("migration not idempotent", people, err)
	}
}

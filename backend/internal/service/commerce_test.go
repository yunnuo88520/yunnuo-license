package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func packageFixture(t *testing.T, s *Service, agent, product string, cost int64, quota bool) domain.CardPackage {
	t.Helper()
	ctx := context.Background()
	p, err := s.CreatePackage(ctx, domain.CardPackage{ProductID: product, Name: "月卡", DurationDays: 30, BaseCost: cost, UseQuota: quota}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantPackages(ctx, GrantPackagesInput{AgentID: agent, PackageID: p.ID, Enabled: true, MaxBatchQuantity: 10, CanExport: true}, "admin"); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestPackagePricingPermissionsTermsAndIdempotency(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	a, err := s.CreateAgent(ctx, CreateAgentInput{Name: "一级代理"})
	if err != nil {
		t.Fatal(err)
	}
	pkg := packageFixture(t, s, a.ID, p.ID, 1000, false)
	tier, err := s.CreateTier(ctx, "金牌", "admin")
	if err != nil {
		t.Fatal(err)
	}
	price := int64(700)
	if err = s.SetTierPrice(ctx, tier.ID, pkg.ID, "admin", &price); err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentTier(ctx, a.ID, tier.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	quotes, err := s.AgentPackages(ctx, a.ID)
	if err != nil || len(quotes) != 1 || quotes[0].UnitCost != 700 || quotes[0].PriceSource != "tier" {
		t.Fatalf("tier price: %+v %v", quotes, err)
	}
	override := int64(500)
	if err = s.GrantPackages(ctx, GrantPackagesInput{AgentID: a.ID, PackageID: pkg.ID, Enabled: true, OverrideCost: &override, MaxBatchQuantity: 10}, "admin"); err != nil {
		t.Fatal(err)
	}
	quotes, _ = s.AgentPackages(ctx, a.ID)
	if quotes[0].UnitCost != 500 || quotes[0].PriceSource != "agent" {
		t.Fatal(quotes)
	}
	in := AgentCreateCardBatchInput{AgentID: a.ID, PackageID: pkg.ID, Quantity: 2, ExpectedUnitCost: &override, RequestID: "issue-001"}
	_, err = s.AgentCreateCardBatch(ctx, in)
	assertCode(t, err, "AGENT_BALANCE_NOT_ENOUGH")
	adjust := AdjustBalanceInput{Amount: 1000, RequestID: "credit-001", Reason: "测试充值"}
	l, err := s.AdjustAgentBalance(ctx, a.ID, "admin", adjust)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.AdjustAgentBalance(ctx, a.ID, "admin", adjust)
	if err != nil || again.ID != l.ID {
		t.Fatal("credit was not idempotent", err)
	}
	in.DurationDays = 999
	_, err = s.AgentCreateCardBatch(ctx, in)
	assertCode(t, err, "PACKAGE_TERMS_MISMATCH")
	in.DurationDays = 0
	wrong := int64(1)
	in.ExpectedUnitCost = &wrong
	_, err = s.AgentCreateCardBatch(ctx, in)
	assertCode(t, err, "PACKAGE_PRICE_CHANGED")
	in.ExpectedUnitCost = &override
	first, err := s.AgentCreateCardBatch(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AgentCreateCardBatch(ctx, in)
	if err != nil || first.Batch.ID != second.Batch.ID || len(second.Codes) != 2 {
		t.Fatal("retry reissued cards", err)
	}
	slices.Sort(first.Codes)
	slices.Sort(second.Codes)
	if !slices.Equal(first.Codes, second.Codes) {
		t.Fatal("retry returned different card codes")
	}
	account, _ := s.AgentAccount(ctx, a.ID)
	ledgers, _ := s.AgentMoneyLedgers(ctx, a.ID)
	if account.Balance != 0 || len(ledgers) != 2 || (ledgers[0].UnitCost != 500 && ledgers[1].UnitCost != 500) {
		t.Fatal("incorrect settlement", account, ledgers)
	}
	in.Quantity = 1
	_, err = s.AgentCreateCardBatch(ctx, in)
	assertCode(t, err, "IDEMPOTENCY_CONFLICT")
	in.RequestID = "legacy-bypass"
	in.PackageID = ""
	in.DurationDays = 30
	in.ProductID = p.ID
	_, err = s.AgentCreateCardBatch(ctx, in)
	assertCode(t, err, "AGENT_PACKAGE_NOT_ALLOWED")
	if err = s.SetPackage(ctx, pkg.ID, "admin", false, 1000); err != nil {
		t.Fatal(err)
	}
	in.PackageID = pkg.ID
	in.RequestID = "disabled-package"
	in.DurationDays = 0
	_, err = s.AgentCreateCardBatch(ctx, in)
	assertCode(t, err, "AGENT_PACKAGE_NOT_ALLOWED")
}
func TestGroupGrantsSnapshotExistingPackages(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	a, _ := s.CreateAgent(ctx, CreateAgentInput{Name: "分组代理"})
	first, err := s.CreatePackage(ctx, domain.CardPackage{ProductID: p.ID, Name: "月卡", DurationDays: 30, BaseCost: 100}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.SaveProductGroup(ctx, domain.ProductGroup{Name: "企业网站", ProductIDs: []string{p.ID}}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantPackages(ctx, GrantPackagesInput{AgentID: a.ID, GroupID: g.ID, Enabled: true, MaxBatchQuantity: 5}, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreatePackage(ctx, domain.CardPackage{ProductID: p.ID, Name: "年卡", DurationDays: 365, BaseCost: 1000}, "admin"); err != nil {
		t.Fatal(err)
	}
	quotes, err := s.AgentPackages(ctx, a.ID)
	if err != nil || len(quotes) != 1 || quotes[0].Package.ID != first.ID {
		t.Fatal("new package automatically granted", quotes, err)
	}
	if err = s.GrantPackages(ctx, GrantPackagesInput{AgentID: a.ID, PackageID: first.ID, Enabled: false, MaxBatchQuantity: 5}, "admin"); err != nil {
		t.Fatal(err)
	}
	quotes, _ = s.AgentPackages(ctx, a.ID)
	if len(quotes) != 0 {
		t.Fatal("revoked package remains available")
	}
}
func TestConcurrentPackageIssuanceCannotOverspend(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	a, _ := s.CreateAgent(ctx, CreateAgentInput{Name: "并发代理"})
	pkg := packageFixture(t, s, a.ID, p.ID, 700, false)
	if _, err := s.AdjustAgentBalance(ctx, a.ID, "admin", AdjustBalanceInput{Amount: 700, RequestID: "credit-concurrent", Reason: "测试"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	cost := int64(700)
	for _, id := range []string{"concurrent-001", "concurrent-002"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := s.AgentCreateCardBatch(ctx, AgentCreateCardBatchInput{AgentID: a.ID, PackageID: pkg.ID, Quantity: 1, ExpectedUnitCost: &cost, RequestID: id})
			results <- err
		}(id)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			assertCode(t, err, "AGENT_BALANCE_NOT_ENOUGH")
		}
	}
	account, _ := s.AgentAccount(ctx, a.ID)
	batches, _ := s.ListAgentCardBatches(ctx, a.ID)
	if success != 1 || account.Balance != 0 || len(batches) != 1 {
		t.Fatalf("overspend: success=%d account=%+v batches=%d", success, account, len(batches))
	}
}
func TestPackageAuditFailureRollsBackBalanceCardsAndLedgers(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "rollback.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
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
	a, _ := s.CreateAgent(ctx, CreateAgentInput{Name: "回滚代理"})
	pkg := packageFixture(t, s, a.ID, p.ID, 500, false)
	if _, err = s.AdjustAgentBalance(ctx, a.ID, "admin", AdjustBalanceInput{Amount: 1000, RequestID: "rollback-credit", Reason: "测试"}); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`CREATE TRIGGER fail_issue_audit BEFORE INSERT ON audit_logs WHEN NEW.action='agent_package.issue' BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	cost := int64(500)
	in := AgentCreateCardBatchInput{AgentID: a.ID, PackageID: pkg.ID, Quantity: 1, RequestID: "rollback-issue", ExpectedUnitCost: &cost}
	if _, err = s.AgentCreateCardBatch(ctx, in); err == nil {
		t.Fatal("audit failure committed issuance")
	}
	after, _ := s.AgentAccount(ctx, a.ID)
	batches, _ := s.ListAgentCardBatches(ctx, a.ID)
	ledgers, _ := s.AgentMoneyLedgers(ctx, a.ID)
	if after.Balance != 1000 || len(batches) != 0 || len(ledgers) != 1 {
		t.Fatal("partial commit", after, batches, ledgers)
	}
	if _, err = raw.Exec(`DROP TRIGGER fail_issue_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AgentCreateCardBatch(ctx, in); err != nil {
		t.Fatal("failed request cannot be retried", err)
	}
}

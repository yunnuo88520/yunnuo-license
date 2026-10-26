package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func TestTierRateAuditFailureRollsBackConfiguration(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "tier-rollback.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	st, err := store.Open(ctx, "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	s := New(st, []byte("pepper"), []byte("0123456789abcdef0123456789abcdef"))
	defer s.Close()
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`CREATE TRIGGER fail_tier_audit BEFORE INSERT ON audit_logs WHEN NEW.action='agent_tier.update' BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveAgentTier(ctx, domain.AgentTier{ID: "tier_gold", Name: "Changed", CostRateBPS: 1, SortOrder: 0}, "admin")
	if err == nil {
		t.Fatal("accepted an unaudited rate change")
	}
	tiers, err := st.Tiers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tier := range tiers {
		if tier.ID == "tier_gold" && (tier.CostRateBPS != 8000 || tier.Name != "金牌代理" || tier.SortOrder != 30) {
			t.Fatal("partial rate change", tier)
		}
	}
}

func TestDefaultAgentTiersAreSeededOnceAndRemainEditable(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	tiers, err := s.currentStore().Tiers(ctx)
	if err != nil || len(tiers) != 3 {
		t.Fatal(tiers, err)
	}
	for i, rate := range []int64{10000, 9000, 8000} {
		if tiers[i].CostRateBPS != rate {
			t.Fatal(tiers)
		}
	}
	standard := tiers[0]
	standard.Name = "普通合作代理"
	standard.CostRateBPS = 8750
	standard.SortOrder = 1
	if _, err = s.SaveAgentTier(ctx, standard, "admin"); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteAgentTier(ctx, "tier_advanced", "admin"); err != nil {
		t.Fatal(err)
	}
	custom, err := s.CreateTier(ctx, "合作伙伴", "admin")
	if err != nil || custom.CostRateBPS != 10000 {
		t.Fatal(custom, err)
	}
	for n := 0; n < 2; n++ {
		if err = s.currentStore().Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
			t.Fatal(err)
		}
	}
	tiers, err = s.currentStore().Tiers(ctx)
	if err != nil || len(tiers) != 3 {
		t.Fatal("defaults resurrected or duplicated", tiers, err)
	}
	if tiers[0].Name != standard.Name || tiers[0].CostRateBPS != 8750 || tiers[0].SortOrder != 1 {
		t.Fatal("edited default was overwritten", tiers)
	}
	for _, v := range tiers {
		if v.ID == "tier_advanced" {
			t.Fatal("deleted default resurrected")
		}
	}
}

func TestTierRatiosPricingPrecedenceHistoryAndReplay(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	a, err := s.CreateAgent(ctx, CreateAgentInput{Name: "Ratio reseller"})
	if err != nil {
		t.Fatal(err)
	}
	pkg := packageFixture(t, s, a.ID, p.ID, 1001, false)
	if err = s.SetAgentTier(ctx, a.ID, "tier_gold", "admin"); err != nil {
		t.Fatal(err)
	}
	check := func(want int64, source string) {
		t.Helper()
		q, e := s.currentStore().Quote(ctx, a.ID, pkg)
		if e != nil || q.UnitCost != want || q.PriceSource != source {
			t.Fatalf("quote=%+v want=%d/%s err=%v", q, want, source, e)
		}
	}
	check(801, "tier_ratio")
	if err = s.DeleteAgentTier(ctx, "tier_gold", "admin"); err == nil {
		t.Fatal("deleted an assigned tier")
	} else {
		assertCode(t, err, "AGENT_TIER_IN_USE")
	}
	if _, err = s.AdjustAgentBalance(ctx, a.ID, "admin", AdjustBalanceInput{Amount: 5000, RequestID: "ratio-credit", Reason: "Opening credit"}); err != nil {
		t.Fatal(err)
	}
	oldCost := int64(801)
	in := AgentCreateCardBatchInput{AgentID: a.ID, PackageID: pkg.ID, Quantity: 1, ExpectedUnitCost: &oldCost, RequestID: "ratio-order-1"}
	first, err := s.AgentCreateCardBatch(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveAgentTier(ctx, domain.AgentTier{ID: "tier_gold", Name: "金牌代理", CostRateBPS: 5000, SortOrder: 30}, "admin"); err != nil {
		t.Fatal(err)
	}
	check(501, "tier_ratio")
	retry, err := s.AgentCreateCardBatch(ctx, in)
	if err != nil || retry.Batch.ID != first.Batch.ID {
		t.Fatal("changed ratio broke replay", retry, err)
	}
	in.RequestID = "ratio-order-2"
	_, err = s.AgentCreateCardBatch(ctx, in)
	assertCode(t, err, "PACKAGE_PRICE_CHANGED")
	newCost := int64(501)
	in.ExpectedUnitCost = &newCost
	if _, err = s.AgentCreateCardBatch(ctx, in); err != nil {
		t.Fatal(err)
	}
	account, err := s.AgentAccount(ctx, a.ID)
	if err != nil || account.Balance != 3698 {
		t.Fatal(account, err)
	}
	ledger, err := s.AgentMoneyLedgers(ctx, a.ID)
	if err != nil || len(ledger) != 3 {
		t.Fatal(ledger, err)
	}
	historical := map[int64]int{}
	for _, l := range ledger {
		if l.Kind == "issue" {
			historical[l.UnitCost]++
		}
	}
	if historical[801] != 1 || historical[501] != 1 {
		t.Fatal("historical price changed", ledger)
	}
	fixed := int64(99)
	if err = s.SetTierPrice(ctx, "tier_gold", pkg.ID, "admin", &fixed); err != nil {
		t.Fatal(err)
	}
	check(99, "tier")
	override := int64(77)
	grant := GrantPackagesInput{AgentID: a.ID, PackageID: pkg.ID, Enabled: true, OverrideCost: &override, MaxBatchQuantity: 10}
	if err = s.GrantPackages(ctx, grant, "admin"); err != nil {
		t.Fatal(err)
	}
	check(77, "agent")
	grant.OverrideCost = nil
	if err = s.GrantPackages(ctx, grant, "admin"); err != nil {
		t.Fatal(err)
	}
	check(99, "tier")
	if err = s.SetTierPrice(ctx, "tier_gold", pkg.ID, "admin", nil); err != nil {
		t.Fatal(err)
	}
	check(501, "tier_ratio")
	if err = s.SetAgentTier(ctx, a.ID, "", "admin"); err != nil {
		t.Fatal(err)
	}
	check(1001, "package")
	if err = s.SetAgentTier(ctx, a.ID, "missing-tier", "admin"); err == nil {
		t.Fatal("assigned missing tier")
	}
}

func TestTierRateBoundsAndIntegerRounding(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	for _, rate := range []int64{-1, 0, 10001, 1 << 62} {
		_, err := s.SaveAgentTier(ctx, domain.AgentTier{Name: "Invalid", CostRateBPS: rate}, "admin")
		assertCode(t, err, "INVALID_REQUEST")
	}
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	a, _ := s.CreateAgent(ctx, CreateAgentInput{Name: "Integer pricing"})
	for _, base := range []int64{0, 1, 1001, maxMoney} {
		pkg := packageFixture(t, s, a.ID, p.ID, base, false)
		if err := s.SetAgentTier(ctx, a.ID, "tier_gold", "admin"); err != nil {
			t.Fatal(err)
		}
		q, err := s.currentStore().Quote(ctx, a.ID, pkg)
		if err != nil || q.UnitCost != (base*8000+9999)/10000 {
			t.Fatal("incorrect integer price", q, err)
		}
	}
}

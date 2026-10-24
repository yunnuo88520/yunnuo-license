package service

import (
	"context"
	"fmt"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"testing"
	"time"
)

func TestHistoryIncludesOldRecordsFiltersNanosecondsAndSeparatesOwners(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	a, _ := s.CreateAgent(ctx, CreateAgentInput{Name: "A"})
	b, _ := s.CreateAgent(ctx, CreateAgentInput{Name: "B"})
	u, _ := s.CreateCustomer(ctx, CustomerInput{Username: "alice", Password: "alice-password"}, "root")
	v, _ := s.CreateCustomer(ctx, CustomerInput{Username: "bob", Password: "bob-password"}, "root")
	batch, cards, _, err := s.buildCardBatch(p, a.ID, "History", 540, 30, false, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.CreateCardBatch(ctx, batch); err != nil {
			return err
		}
		for i, card := range cards {
			if err := tx.CreateCard(ctx, card); err != nil {
				return err
			}
			stamp := s.now().Add(-time.Duration(i) * time.Minute)
			lic := newLicense(p, card, stamp)
			lic.OwnerUserID = u.ID
			if i%2 == 1 {
				lic.OwnerUserID = v.ID
			}
			if err := tx.CreateLicense(ctx, lic); err != nil {
				return err
			}
			if _, err := tx.RecordSecurityCase(ctx, domain.SecurityCase{ID: fmt.Sprintf("case_%04d", i), ProductID: p.ID, LicenseID: lic.ID, Signal: "integrity_mismatch", Subject: "1.0.0", Evidence: "private-evidence", Status: "open", Action: "observe", Occurrences: 1, FirstSeen: stamp, LastSeen: stamp}); err != nil {
				return err
			}
			remark := "normal"
			if i == 539 {
				remark = "oldest-marker"
			}
			if i == 0 {
				stamp = stamp.Add(123 * time.Nanosecond)
			}
			if err := tx.InsertMoney(ctx, domain.MoneyLedger{ID: fmt.Sprintf("money_%04d", i), AgentID: a.ID, RequestID: yncrypto.NewID("req"), Kind: "adjust", Amount: 1, BalanceAfter: int64(i + 1), Remark: remark, CreatedAt: stamp}); err != nil {
				return err
			}
		}
		for _, agent := range []string{a.ID, b.ID} {
			if err := tx.InsertMoney(ctx, domain.MoneyLedger{ID: yncrypto.NewID("money"), AgentID: agent, RequestID: yncrypto.NewID("req"), Kind: "adjust", Amount: 1, BalanceAfter: 1, CreatedAt: s.now()}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.History(ctx, "money", "agent", a.ID, domain.HistoryFilter{Page: 6, PageSize: 100, AgentID: b.ID})
	if err != nil || page.Total != 541 || len(page.Items) != 41 {
		t.Fatalf("old history/RLS: total=%d rows=%d err=%v", page.Total, len(page.Items), err)
	}
	found, err := s.History(ctx, "money", "admin", "", domain.HistoryFilter{Query: "oldest-marker"})
	if err != nil || found.Total != 1 {
		t.Fatal("oldest record cannot be searched", found, err)
	}
	at := s.now()
	window, err := s.History(ctx, "money", "agent", a.ID, domain.HistoryFilter{From: at.In(time.FixedZone("east", 8*3600)).Format(time.RFC3339), To: at.Add(time.Second).Format(time.RFC3339)})
	if err != nil || window.Total != 2 {
		t.Fatal("time boundary excluded fractional-second records", window.Total, err)
	}
	count := 0
	if err = s.StreamHistory(ctx, "money", "agent", a.ID, domain.HistoryFilter{}, func(any) error { count++; return nil }); err != nil || count != 541 {
		t.Fatal("export truncated history", count, err)
	}
	owned, err := s.History(ctx, "cases", "customer", u.ID, domain.HistoryFilter{Page: 14, PageSize: 20})
	if err != nil || owned.Total != 270 || len(owned.Items) != 10 {
		t.Fatal("customer history pagination", owned.Total, len(owned.Items), err)
	}
	for _, item := range owned.Items {
		if item.(domain.SecurityCase).Evidence != "" {
			t.Fatal("private evidence leaked")
		}
	}
	hidden, err := s.History(ctx, "cases", "customer", u.ID, domain.HistoryFilter{Query: "private-evidence"})
	if err != nil || hidden.Total != 0 {
		t.Fatal("private evidence searchable by customer")
	}
}

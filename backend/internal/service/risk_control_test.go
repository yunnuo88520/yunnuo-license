package service

import (
	"context"
	"testing"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func TestRiskBlockRejectsActivationAndCanBeDisabled(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	code := createTestCard(t, ctx, svc, product.ID, 30)

	block, err := svc.CreateRiskBlock(ctx, CreateRiskBlockInput{
		ProductID: product.ID,
		Kind:      domain.RiskBlockIP,
		Value:     "203.0.113.10",
		Reason:    "automated abuse test",
		ActorID:   "admin-test",
	})
	if err != nil {
		t.Fatalf("create risk block: %v", err)
	}
	if block.ValueMasked == "203.0.113.10" || block.ValueHash == "" {
		t.Fatalf("risk block did not protect target value: %#v", block)
	}

	activate := func() error {
		_, err := svc.Activate(ctx, ActivateInput{
			AppKey: product.AppKey, CardCode: code, BindMode: domain.BindDevice,
			BindValue: "risk-test-device", ClientIP: "203.0.113.10", UserAgent: "risk-test",
		})
		return err
	}
	assertCode(t, activate(), "RISK_IP_BLOCKED")
	assertCode(t, activate(), "RISK_IP_BLOCKED")

	alerts, err := svc.ListRiskAlerts(ctx, RiskAlertFilter{Status: domain.RiskAlertOpen})
	if err != nil || len(alerts) != 1 || alerts[0].AlertType != "blocked_ip" || alerts[0].OccurrenceCount != 2 {
		t.Fatalf("unexpected blocked access alert: alerts=%#v err=%v", alerts, err)
	}
	if alerts[0].SubjectMasked == "203.0.113.10" {
		t.Fatal("risk alert leaked full IP address")
	}
	if err := svc.ResolveRiskAlert(ctx, alerts[0].ID, "admin-test"); err != nil {
		t.Fatalf("resolve blocked access alert: %v", err)
	}
	assertCode(t, activate(), "RISK_IP_BLOCKED")
	alerts, err = svc.ListRiskAlerts(ctx, RiskAlertFilter{Status: domain.RiskAlertOpen})
	if err != nil || len(alerts) != 1 || alerts[0].OccurrenceCount != 1 || alerts[0].ResolvedAt != nil {
		t.Fatalf("expected a new open alert after resolution: alerts=%#v err=%v", alerts, err)
	}

	if err := svc.DisableRiskBlock(ctx, block.ID, "admin-test"); err != nil {
		t.Fatalf("disable risk block: %v", err)
	}
	if err := activate(); err != nil {
		t.Fatalf("activation should recover after disabling block: %v", err)
	}
}

func TestRepeatedDeviceActivationsCreateAggregatedAlert(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	batch, err := svc.CreateCardBatch(ctx, CreateCardBatchInput{
		ProductID: product.ID, Name: "risk devices", Quantity: 4, DurationDays: 30,
	})
	if err != nil {
		t.Fatalf("create risk test cards: %v", err)
	}
	for i, code := range batch.Codes {
		if _, err := svc.Activate(ctx, ActivateInput{
			AppKey: product.AppKey, CardCode: code, BindMode: domain.BindDevice,
			BindValue: "shared-device-fingerprint", ClientIP: "198.51.100.20",
		}); err != nil {
			t.Fatalf("activate card %d: %v", i, err)
		}
	}
	alerts, err := svc.ListRiskAlerts(ctx, RiskAlertFilter{Status: domain.RiskAlertOpen, AlertType: "device_multi_license"})
	if err != nil || len(alerts) != 1 {
		t.Fatalf("list device alerts: alerts=%#v err=%v", alerts, err)
	}
	if alerts[0].OccurrenceCount != 2 || alerts[0].Severity != "high" {
		t.Fatalf("expected aggregated third and fourth activation alert, got %#v", alerts[0])
	}
	if err := svc.ResolveRiskAlert(ctx, alerts[0].ID, "admin-test"); err != nil {
		t.Fatalf("resolve alert: %v", err)
	}
	summary, err := svc.RiskSummary(ctx)
	if err != nil || summary.OpenAlerts != 0 || summary.Alerts24Hours != 1 {
		t.Fatalf("unexpected risk summary: summary=%#v err=%v", summary, err)
	}
}

func TestActivationFailureBurstAutoBlocksIP(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)

	probe := func() error {
		code, err := yncrypto.GenerateCardCode(product.Code)
		if err != nil {
			t.Fatalf("generate probe code: %v", err)
		}
		_, err = svc.Activate(ctx, ActivateInput{
			AppKey: product.AppKey, CardCode: code, BindMode: domain.BindDevice,
			BindValue: "auto-block-device", ClientIP: "203.0.113.77",
		})
		return err
	}

	for i := 0; i < activationAutoBlockThreshold-1; i++ {
		assertCode(t, probe(), "CARD_INVALID")
	}
	blocks, err := svc.ListRiskBlocks(ctx, RiskBlockFilter{Kind: domain.RiskBlockIP, Status: domain.RiskStatusActive})
	if err != nil || len(blocks) != 0 {
		t.Fatalf("expected no auto block below threshold: blocks=%#v err=%v", blocks, err)
	}
	alerts, err := svc.ListRiskAlerts(ctx, RiskAlertFilter{Status: domain.RiskAlertOpen, AlertType: "activation_failure_burst"})
	if err != nil || len(alerts) != 1 {
		t.Fatalf("expected failure burst alert before auto block: alerts=%#v err=%v", alerts, err)
	}

	assertCode(t, probe(), "CARD_INVALID")
	blocks, err = svc.ListRiskBlocks(ctx, RiskBlockFilter{Kind: domain.RiskBlockIP, Status: domain.RiskStatusActive})
	if err != nil || len(blocks) != 1 || blocks[0].ProductID != product.ID {
		t.Fatalf("expected product-scoped auto block: blocks=%#v err=%v", blocks, err)
	}
	alerts, err = svc.ListRiskAlerts(ctx, RiskAlertFilter{Status: domain.RiskAlertOpen, AlertType: "activation_auto_blocked"})
	if err != nil || len(alerts) != 1 || alerts[0].Severity != "critical" {
		t.Fatalf("expected critical auto-block alert: alerts=%#v err=%v", alerts, err)
	}
	assertCode(t, probe(), "RISK_IP_BLOCKED")
}

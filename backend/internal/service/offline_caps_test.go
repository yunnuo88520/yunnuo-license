package service

import (
	"context"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func TestOfflineDurationAndGraceCaps(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)

	_, err := svc.CreateOfflineLicense(ctx, CreateOfflineLicenseInput{
		ProductID: product.ID, MachineCode: "MC-CAP-TEST-001", DurationDays: 366,
	}, "admin-test")
	assertCode(t, err, "OFFLINE_DURATION_TOO_LONG")

	created, err := svc.CreateOfflineLicense(ctx, CreateOfflineLicenseInput{
		ProductID: product.ID, MachineCode: "MC-CAP-TEST-001", DurationDays: 365,
	}, "admin-test")
	if err != nil {
		t.Fatalf("365-day offline license should be accepted: %v", err)
	}
	if created.ExpiredAt == nil {
		t.Fatal("duration license must carry an expiry")
	}

	_, err = svc.UpdateProductProfile(ctx, product.ID, UpdateProductInput{
		Name: "Test Product", BindMode: domain.BindDevice, OfflineGraceDays: 366,
	}, "admin-test")
	assertCode(t, err, "OFFLINE_GRACE_TOO_LONG")

	if _, err := svc.UpdateProductProfile(ctx, product.ID, UpdateProductInput{
		Name: "Test Product", BindMode: domain.BindDevice, OfflineGraceDays: 30,
	}, "admin-test"); err != nil {
		t.Fatalf("30-day grace should be accepted: %v", err)
	}
}

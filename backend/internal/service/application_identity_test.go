package service

import (
	"context"
	"regexp"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func TestApplicationIdentityIsGeneratedUniqueAndStable(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	first, err := s.CreateProduct(ctx, CreateProductInput{Name: "Ebox", BindMode: domain.BindDomain})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateProduct(ctx, CreateProductInput{Name: "Ebox", BindMode: domain.BindDomain})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.Code == second.Code || !regexp.MustCompile(`^yn[0-9a-f]{16}$`).MatchString(first.ID) || first.AppKey != first.ID || second.AppKey != second.ID {
		t.Fatalf("applications must have unique generated IDs, independent of their names: %q %q", first.ID, second.ID)
	}
	card := createTestCard(t, ctx, s, first.ID, 30)
	license, err := s.Activate(ctx, ActivateInput{AppKey: first.ID, CardCode: card, BindValue: "ebox.example.com"})
	if err != nil || !license.CanUse {
		t.Fatalf("the displayed application ID must work directly in the SDK's app_key field: %v", err)
	}
	updated, err := s.UpdateProductProfile(ctx, first.ID, UpdateProductInput{Name: "Ebox 企业版", BindMode: domain.BindDomain, MaxBindCount: 1, OfflineGraceDays: 15}, "admin")
	if err != nil || updated.ID != first.ID || updated.Code != first.Code || updated.AppKey != first.ID {
		t.Fatalf("changing the name changed the application identity: %v", err)
	}
	if _, err = s.Verify(ctx, VerifyInput{AppKey: first.ID, LicenseNo: license.LicenseNo, BindValue: "ebox.example.com"}); err != nil {
		t.Fatalf("renaming invalidated the existing license: %v", err)
	}
	_, err = s.Verify(ctx, VerifyInput{AppKey: second.ID, LicenseNo: license.LicenseNo, BindValue: "ebox.example.com"})
	assertCode(t, err, "LICENSE_NOT_FOUND")
}

func TestCustomerActivationDetectsApplicationAndRejectsConflictingSelection(t *testing.T) {
	ctx := context.Background()
	s, product, alice, bob, oldCard, _ := customerFixture(t)
	card := createTestCard(t, ctx, s, product.ID, 30)
	input := ActivateInput{CardCode: card, BindValue: "automatic.example.com"}
	_, err := s.CustomerActivate(ctx, alice.ID, "prod_unrelated", input)
	assertCode(t, err, "CARD_INVALID")
	license, err := s.CustomerActivate(ctx, alice.ID, "", input)
	if err != nil || !license.CanUse {
		t.Fatalf("card-only activation failed: %v", err)
	}
	owned, err := s.CustomerLicense(ctx, alice.ID, license.LicenseNo)
	if err != nil || owned.Product.ID != product.ID {
		t.Fatalf("card-only activation selected the wrong product: %v", err)
	}
	_, err = s.CustomerActivate(ctx, bob.ID, "", ActivateInput{CardCode: oldCard, BindValue: "old.example.com"})
	assertCode(t, err, "OWNER_REQUIRED")
	_, err = s.CustomerActivate(ctx, alice.ID, "", ActivateInput{CardCode: "unknown-card"})
	assertCode(t, err, "CARD_INVALID")
	_, err = s.ChangeProductStatus(ctx, product.ID, domain.ProductDisabled, "admin")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CustomerActivate(ctx, alice.ID, "", input)
	assertCode(t, err, "PRODUCT_DISABLED")
}

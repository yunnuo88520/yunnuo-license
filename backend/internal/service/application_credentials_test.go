package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func TestApplicationSecretLifecycleAndIndependentLicenseRights(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p, err := s.CreateProduct(ctx, CreateProductInput{Name: "Ebox", BindMode: domain.BindDomain})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^yn[0-9a-f]{16}$`).MatchString(p.ID) || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(p.AppSecret) {
		t.Fatal("unexpected AppID/AppSecret format")
	}
	status, err := s.ApplicationCredentialStatus(ctx, p.ID)
	if err != nil || !status.Configured || status.Version != 1 {
		t.Fatal("credential not created atomically", err)
	}
	stored, err := s.currentStore().ApplicationCredential(ctx, p.ID)
	if err != nil || stored.SecretHash == p.AppSecret || strings.Contains(stored.SecretHash, p.AppSecret) {
		t.Fatal("plaintext secret stored", err)
	}
	products, err := s.ListProducts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(products)
	if strings.Contains(string(raw), p.AppSecret) || strings.Contains(string(raw), "app_secret") {
		t.Fatal("product list leaked a secret")
	}
	in := ApplicationTokenInput{GrantType: "client_credential", AppID: p.ID, Secret: p.AppSecret}
	token, err := s.ApplicationToken(ctx, in)
	if err != nil || token.ExpiresIn != 7200 {
		t.Fatal("cannot exchange valid credentials", err)
	}
	if got, err := s.AuthenticateApplicationToken(ctx, token.AccessToken); err != nil || got.ID != p.ID || got.AppSecret != "" {
		t.Fatal("invalid token identity", err)
	}
	for _, value := range []string{"", token.AccessToken + "x", "app1.invalid.invalid"} {
		_, err = s.AuthenticateApplicationToken(ctx, value)
		assertCode(t, err, "INVALID_APP_ACCESS_TOKEN")
	}
	other, err := s.CreateProduct(ctx, CreateProductInput{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ApplicationToken(ctx, ApplicationTokenInput{GrantType: "client_credential", AppID: other.ID, Secret: p.AppSecret})
	assertCode(t, err, "INVALID_APP_CREDENTIALS")
	card := createTestCard(t, ctx, s, p.ID, 30)
	license, err := s.Activate(ctx, ActivateInput{AppKey: p.ID, CardCode: card, BindValue: "ebox.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	reset, err := s.ResetApplicationSecret(ctx, p.ID, "root", ResetApplicationSecretInput{ExpectedVersion: 1, Reason: "定期更换密钥"})
	if err != nil || reset.Version != 2 || reset.AppSecret == p.AppSecret || reset.AppID != p.ID {
		t.Fatal("reset failed", err)
	}
	_, err = s.AuthenticateApplicationToken(ctx, token.AccessToken)
	assertCode(t, err, "INVALID_APP_ACCESS_TOKEN")
	_, err = s.ApplicationToken(ctx, in)
	assertCode(t, err, "INVALID_APP_CREDENTIALS")
	in.Secret = reset.AppSecret
	token, err = s.ApplicationToken(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ResetApplicationSecret(ctx, p.ID, "root", ResetApplicationSecretInput{ExpectedVersion: 1, Reason: "过时的重置请求"})
	assertCode(t, err, "APP_SECRET_CHANGED")
	if _, err = s.Verify(ctx, VerifyInput{AppKey: p.ID, LicenseNo: license.LicenseNo, BindValue: "ebox.example.com"}); err != nil {
		t.Fatal("AppSecret reset changed end-user license rights", err)
	}
	if err = s.currentStore().Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateApplicationToken(ctx, token.AccessToken); err != nil {
		t.Fatal("repeat migration reset application credentials", err)
	}
	now := s.now()
	s.now = func() time.Time { return now.Add(2 * time.Hour) }
	_, err = s.AuthenticateApplicationToken(ctx, token.AccessToken)
	assertCode(t, err, "INVALID_APP_ACCESS_TOKEN")
	s.now = func() time.Time { return now }
	_, err = s.ChangeProductStatus(ctx, p.ID, domain.ProductDisabled, "root")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateApplicationToken(ctx, token.AccessToken)
	assertCode(t, err, "INVALID_APP_ACCESS_TOKEN")
	_, err = s.ApplicationToken(ctx, in)
	assertCode(t, err, "INVALID_APP_CREDENTIALS")
}

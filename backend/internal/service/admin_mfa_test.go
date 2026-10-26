package service

import (
	"context"
	"encoding/base32"
	"errors"
	"testing"
	"time"
)

func TestTOTPRFC6238SHA1(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, c := range []struct {
		at   int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		if got := totp(secret, c.at/30); got != c.want {
			t.Fatalf("%d: %s", c.at, got)
		}
	}
}
func TestAdminMFALifecycleRevokesSessionsAndRejectsReplayedCodes(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	user, _, err := s.EnsureBootstrapAdmin(ctx, "root", "test-password", "Root")
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginAdminMFA(ctx, user.ID, "wrong"); err == nil {
		t.Fatal("accepted password")
	}
	setup, err := s.BeginAdminMFA(ctx, user.ID, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	codes, err := s.ConfirmAdminMFA(ctx, user.ID, "test-password", totp(setup["secret"], s.now().Unix()/30))
	if err != nil || len(codes) != 10 {
		t.Fatal(codes, err)
	}
	if _, err = s.AuthenticateAdminToken(ctx, old.AccessToken); err == nil {
		t.Fatal("old session survived enable")
	}
	for _, code := range []string{"", "000000", totp(setup["secret"], s.now().Unix()/30)} {
		_, err = s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password", OTPCode: code})
		if !errors.Is(err, ErrMFARequired) {
			t.Fatal("accepted missing/wrong/replayed code", err)
		}
	}
	login, err := s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password", OTPCode: codes[0]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password", OTPCode: codes[0]}); err == nil {
		t.Fatal("recovery code reused")
	}
	base := s.now()
	s.now = func() time.Time { return base.Add(30 * time.Second) }
	if _, err = s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password", OTPCode: totp(setup["secret"], s.now().Unix()/30)}); err != nil {
		t.Fatal(err)
	}
	if err = s.DisableAdminMFA(ctx, user.ID, "test-password", codes[1]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateAdminToken(ctx, login.AccessToken); err == nil {
		t.Fatal("session survived disable")
	}
	if _, err = s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password"}); err != nil {
		t.Fatal(err)
	}
}

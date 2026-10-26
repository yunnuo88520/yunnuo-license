package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newMFALoginFixture(t *testing.T) (*Service, string, []string) {
	t.Helper()
	ctx := context.Background()
	s := newTestService(t)
	user, _, err := s.EnsureBootstrapAdmin(ctx, "root", "test-password", "Root")
	if err != nil {
		t.Fatal(err)
	}
	setup, err := s.BeginAdminMFA(ctx, user.ID, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	codes, err := s.ConfirmAdminMFA(ctx, user.ID, "test-password", totp(setup["secret"], s.now().Unix()/30))
	if err != nil {
		t.Fatal(err)
	}
	return s, setup["secret"], codes
}

func TestAdminLoginOnlyRequestsSecondStepAfterPasswordAndOptIn(t *testing.T) {
	ctx := context.Background()
	input := AdminLoginInput{Username: "root", Password: "test-password"}
	s := newTestService(t)
	s.EnsureBootstrapAdmin(ctx, input.Username, input.Password, "Root")
	out, err := s.StartAdminLogin(ctx, input, "192.0.2.1")
	if err != nil || out.Login == nil || out.MFA != nil || out.Login.AccessToken == "" {
		t.Fatal("ordinary account should log in directly", err)
	}
	s, _, _ = newMFALoginFixture(t)
	out, err = s.StartAdminLogin(ctx, AdminLoginInput{Username: "root", Password: "wrong"}, "192.0.2.1")
	if !errors.Is(err, ErrInvalidCredentials) || out.MFA != nil || out.Login != nil {
		t.Fatal("wrong password exposed a second step", err)
	}
	out, err = s.StartAdminLogin(ctx, input, "192.0.2.1")
	if err != nil || out.MFA == nil || out.Login != nil {
		t.Fatal("enabled account did not require second step", err)
	}
	if _, err = s.AuthenticateAdminToken(ctx, out.MFA.Challenge); err == nil {
		t.Fatal("challenge was accepted as a logged-in session")
	}
}

func TestAdminLoginSecondStepDoesNotReuseConsumedImageCaptcha(t *testing.T) {
	ctx := context.Background()
	s, _, codes := newMFALoginFixture(t)
	if _, err := s.UpdateCaptchaSettings(ctx, UpdateCaptchaSettingsInput{LoginEnabled: true}); err != nil {
		t.Fatal(err)
	}
	input := AdminLoginInput{Username: "root", Password: "test-password"}
	_, err := s.StartAdminLogin(ctx, input, "192.0.2.1")
	assertCode(t, err, "CAPTCHA_REQUIRED")
	if err := s.currentStore().CreateCaptchaChallenge(ctx, "cap_login", recoveryHash("ABCD"), s.now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	input.CaptchaID, input.CaptchaCode = "cap_login", "abcd"
	out, err := s.StartAdminLogin(ctx, input, "192.0.2.1")
	if err != nil || out.MFA == nil {
		t.Fatal(err)
	}
	_, err = s.StartAdminLogin(ctx, input, "192.0.2.1")
	assertCode(t, err, "CAPTCHA_INVALID")
	_, err = s.CompleteAdminLogin(ctx, CompleteAdminLoginInput{Challenge: out.MFA.Challenge, Code: "invalid"}, "192.0.2.1")
	assertCode(t, err, "MFA_CODE_INVALID")
	login, err := s.CompleteAdminLogin(ctx, CompleteAdminLoginInput{Challenge: out.MFA.Challenge, Code: codes[0]}, "192.0.2.1")
	if err != nil || login.AccessToken == "" {
		t.Fatal("second step required another CAPTCHA or password", err)
	}
	_, err = s.CompleteAdminLogin(ctx, CompleteAdminLoginInput{Challenge: out.MFA.Challenge, Code: codes[1]}, "192.0.2.1")
	assertCode(t, err, "MFA_CHALLENGE_INVALID")
}

func TestAdminLoginChallengeBindingExpiryAndRevocation(t *testing.T) {
	for _, reason := range []string{"ip", "signature", "expiry", "password", "disabled", "legacy-login"} {
		t.Run(reason, func(t *testing.T) {
			ctx := context.Background()
			s, _, codes := newMFALoginFixture(t)
			out, err := s.StartAdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password"}, "192.0.2.1")
			if err != nil {
				t.Fatal(err)
			}
			input := CompleteAdminLoginInput{Challenge: out.MFA.Challenge, Code: codes[0]}
			ip, want := "192.0.2.1", "MFA_CHALLENGE_INVALID"
			user, err := s.currentStore().GetAdminUserByUsername(ctx, "root")
			if err != nil {
				t.Fatal(err)
			}
			switch reason {
			case "ip":
				ip = "192.0.2.2"
			case "signature":
				input.Challenge = "x" + input.Challenge
			case "expiry":
				s.now = func() time.Time { return out.MFA.ExpiresAt }
				want = "MFA_CHALLENGE_EXPIRED"
			case "password":
				err = s.ChangeAdminPassword(ctx, user.ID, ChangeAdminPasswordInput{CurrentPassword: "test-password", NewPassword: "new-password"})
			case "disabled":
				err = s.DisableAdminMFA(ctx, user.ID, "test-password", codes[1])
			case "legacy-login":
				_, err = s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password", OTPCode: codes[1]})
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.CompleteAdminLogin(ctx, input, ip)
			assertCode(t, err, want)
		})
	}
}

func TestAdminLoginChallengeIsSingleUseEvenWithDifferentValidFactors(t *testing.T) {
	ctx := context.Background()
	s, secret, codes := newMFALoginFixture(t)
	out, err := s.StartAdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password"}, "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, code := range codes[:2] {
		go func(code string) {
			_, err := s.CompleteAdminLogin(ctx, CompleteAdminLoginInput{Challenge: out.MFA.Challenge, Code: code}, "192.0.2.1")
			results <- err
		}(code)
	}
	winners := 0
	for range 2 {
		if err := <-results; err == nil {
			winners++
		} else if !errors.Is(err, ErrMFALoginInvalid) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatal("concurrent completions", winners)
	}
	user, _ := s.currentStore().GetAdminUserByUsername(ctx, "root")
	status, _ := s.MFAStatus(ctx, user.ID)
	if status["recovery_remaining"] != 9 {
		t.Fatal("consumed more than one recovery code", status)
	}
	base := s.now()
	s.now = func() time.Time { return base.Add(30 * time.Second) }
	out, err = s.StartAdminLogin(ctx, AdminLoginInput{Username: "root", Password: "test-password"}, "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteAdminLogin(ctx, CompleteAdminLoginInput{Challenge: out.MFA.Challenge, Code: totp(secret, s.now().Unix()/30)}, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return base.Add(time.Minute) }
	_, err = s.CompleteAdminLogin(ctx, CompleteAdminLoginInput{Challenge: out.MFA.Challenge, Code: totp(secret, s.now().Unix()/30)}, "192.0.2.1")
	assertCode(t, err, "MFA_CHALLENGE_INVALID")
}

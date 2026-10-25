package service

import (
	"context"
	"strings"
	"testing"
)

func TestLoginCaptchaFlow(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	if _, _, err := s.EnsureBootstrapAdmin(ctx, "root", "root-password", "Root"); err != nil {
		t.Fatal(err)
	}

	// 默认关闭：直接登录放行。
	if _, err := s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "root-password"}); err != nil {
		t.Fatalf("login should pass while disabled: %v", err)
	}

	// 开启后：缺验证码被拦截。
	if _, err := s.UpdateCaptchaSettings(ctx, UpdateCaptchaSettingsInput{LoginEnabled: true, ActorID: "root"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "root-password"})
	assertCode(t, err, "CAPTCHA_REQUIRED")

	// 错误验证码被拒绝。
	_, err = s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "root-password", CaptchaID: "cap_none", CaptchaCode: "ABCD"})
	assertCode(t, err, "CAPTCHA_INVALID")

	// 正确验证码通过（直接向存储写入已知哈希的挑战）。
	if err := s.currentStore().CreateCaptchaChallenge(ctx, "cap_ok", recoveryHash("ABCD"), s.now().Add(5*60*1e9)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "root-password", CaptchaID: "cap_ok", CaptchaCode: "abcd"}); err != nil {
		t.Fatalf("login with valid captcha should pass: %v", err)
	}

	// 同一验证码只能使用一次。
	_, err = s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "root-password", CaptchaID: "cap_ok", CaptchaCode: "ABCD"})
	assertCode(t, err, "CAPTCHA_INVALID")

	// 关闭开关后恢复放行。
	if _, err := s.UpdateCaptchaSettings(ctx, UpdateCaptchaSettingsInput{LoginEnabled: false, ActorID: "root"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminLogin(ctx, AdminLoginInput{Username: "root", Password: "root-password"}); err != nil {
		t.Fatalf("login should pass after disabling: %v", err)
	}
}

func TestNewCaptchaShape(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	challenge, err := s.NewCaptcha(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(challenge.ID, "cap_") {
		t.Fatalf("unexpected challenge id %q", challenge.ID)
	}
	if !strings.HasPrefix(challenge.Image, "data:image/png;base64,") || len(challenge.Image) < 500 {
		t.Fatalf("image payload too small or malformed: %d bytes", len(challenge.Image))
	}
}

package service

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	yncaptcha "github.com/yunnuo88520/yunnuo-license/backend/internal/captcha"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

const captchaTTL = 5 * time.Minute

type CaptchaSettingsView struct {
	LoginEnabled bool      `json:"login_enabled"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UpdateCaptchaSettingsInput struct {
	LoginEnabled bool   `json:"login_enabled"`
	ActorID      string `json:"-"`
}

func (s *Service) CaptchaSettings(ctx context.Context) (CaptchaSettingsView, error) {
	settings, err := s.currentStore().GetCaptchaSettings(ctx)
	if err != nil {
		return CaptchaSettingsView{}, err
	}
	return CaptchaSettingsView{LoginEnabled: settings.LoginEnabled == 1, UpdatedAt: settings.UpdatedAt}, nil
}

func (s *Service) UpdateCaptchaSettings(ctx context.Context, input UpdateCaptchaSettingsInput) (CaptchaSettingsView, error) {
	settings := domain.CaptchaSettings{
		LoginEnabled: boolToInt(input.LoginEnabled),
		UpdatedAt:    s.now(),
	}
	if err := s.currentStore().UpdateCaptchaSettings(ctx, settings); err != nil {
		return CaptchaSettingsView{}, err
	}
	_ = s.audit(ctx, "admin", input.ActorID, "", "", "", "captcha_settings.update", "success", "")
	return CaptchaSettingsView{LoginEnabled: input.LoginEnabled, UpdatedAt: settings.UpdatedAt}, nil
}

// NewCaptcha 签发一个图形验证码：挑战码只存哈希，图片以 data URL 返回。
func (s *Service) NewCaptcha(ctx context.Context) (domain.CaptchaChallenge, error) {
	// 顺带清理过期与已用挑战，避免表无限增长。
	_ = s.currentStore().CleanupCaptchaChallenges(ctx, s.now())
	code, err := yncaptcha.NewCode()
	if err != nil {
		return domain.CaptchaChallenge{}, err
	}
	pngBytes, err := yncaptcha.Render(code)
	if err != nil {
		return domain.CaptchaChallenge{}, err
	}
	expiresAt := s.now().Add(captchaTTL)
	id := yncrypto.NewID("cap")
	if err := s.currentStore().CreateCaptchaChallenge(ctx, id, recoveryHash(code), expiresAt); err != nil {
		return domain.CaptchaChallenge{}, err
	}
	return domain.CaptchaChallenge{
		ID:        id,
		Image:     "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes),
		ExpiresAt: expiresAt,
	}, nil
}

// ensureLoginCaptcha 在登录流程前置校验图形验证码；开关关闭时直接放行。
func (s *Service) ensureLoginCaptcha(ctx context.Context, id, answer string) error {
	settings, err := s.currentStore().GetCaptchaSettings(ctx)
	if err != nil {
		return err
	}
	if settings.LoginEnabled != 1 {
		return nil
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(answer) == "" {
		return unauthorized("CAPTCHA_REQUIRED", "请输入图形验证码")
	}
	ok, err := s.currentStore().ConsumeCaptchaChallenge(ctx, strings.TrimSpace(id), recoveryHash(strings.ToUpper(strings.TrimSpace(answer))), s.now())
	if err != nil {
		return err
	}
	if !ok {
		return unauthorized("CAPTCHA_INVALID", "图形验证码错误或已失效，请重试")
	}
	return nil
}

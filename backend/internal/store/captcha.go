package store

import (
	"context"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func (s *Store) GetCaptchaSettings(ctx context.Context) (domain.CaptchaSettings, error) {
	var settings domain.CaptchaSettings
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT login_enabled, updated_at FROM captcha_settings WHERE id = 1`).Scan(
		&settings.LoginEnabled, &updatedAt,
	)
	if err != nil {
		return domain.CaptchaSettings{}, err
	}
	settings.UpdatedAt = parseTS(updatedAt)
	return settings, nil
}

func (s *Store) UpdateCaptchaSettings(ctx context.Context, settings domain.CaptchaSettings) error {
	_, err := s.db.ExecContext(ctx, `UPDATE captcha_settings SET login_enabled = ?, updated_at = ? WHERE id = 1`,
		settings.LoginEnabled, ts(settings.UpdatedAt))
	return err
}

func (s *Store) CreateCaptchaChallenge(ctx context.Context, id, codeHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO captcha_challenges (id, code_hash, expires_at) VALUES (?,?,?)`,
		id, codeHash, ts(expiresAt))
	return err
}

// ConsumeCaptchaChallenge 原子消费一次验证码：匹配哈希、未过期、未使用三者
// 同时满足才返回 true，无论对错每个验证码只能尝试一次。
func (s *Store) ConsumeCaptchaChallenge(ctx context.Context, id, codeHash string, now time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE captcha_challenges SET consumed_at = ? WHERE id = ? AND code_hash = ? AND consumed_at IS NULL AND expires_at > ?`,
		ts(now), id, codeHash, ts(now))
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// CleanupCaptchaChallenges 清理过期或已使用的旧挑战，签发新验证码时顺带执行。
func (s *Store) CleanupCaptchaChallenges(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM captcha_challenges WHERE consumed_at IS NOT NULL OR expires_at <= ?`, ts(now.Add(-time.Hour)))
	return err
}

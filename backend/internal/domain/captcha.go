package domain

import "time"

// CaptchaSettings 单行表：登录图形验证码开关。
type CaptchaSettings struct {
	LoginEnabled int       `json:"login_enabled"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CaptchaChallenge 预置给管理端的签发结果（挑战码本体只在服务端存哈希）。
type CaptchaChallenge struct {
	ID        string    `json:"id"`
	Image     string    `json:"image"`
	ExpiresAt time.Time `json:"expires_at"`
}

package domain

import "time"

type RecoveryAccount struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	PasswordHash string `json:"-"`
}
type PasswordReset struct {
	ID, TokenHash, Kind, UserID, CredentialHash, CreatedBy, Reason string
	ExpiresAt, CreatedAt                                           time.Time
	ConsumedAt                                                     *time.Time
}

// ResetAccountRef 按用户名定位到的账号引用，用于 6 位验证码的作用域查找。
type ResetAccountRef struct {
	ID       string
	Username string
	Kind     string
	Status   string
}

package domain

import "time"

type RecoveryAccount struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	PasswordHash string `json:"-"`
}
type PasswordReset struct {
	ID, TokenHash, Kind, UserID, CredentialHash, CreatedBy, Reason string
	ExpiresAt, CreatedAt                                           time.Time
	ConsumedAt                                                     *time.Time
}

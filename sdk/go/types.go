package ynlicense

import "time"

type ActivateRequest struct {
	ClientInstance
	CardCode      string `json:"card_code"`
	BindMode      string `json:"bind_mode"`
	BindValue     string `json:"bind_value"`
	DeviceName    string `json:"device_name,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
}

type VerifyRequest struct {
	ClientInstance
	LicenseNo    string `json:"license_no"`
	BindMode     string `json:"bind_mode"`
	BindValue    string `json:"bind_value"`
	LicenseToken string `json:"license_token,omitempty"`
}

type HeartbeatRequest struct {
	ClientInstance
	LicenseNo string `json:"license_no"`
	BindMode  string `json:"bind_mode"`
	BindValue string `json:"bind_value"`
}

type RenewRequest struct {
	ClientInstance
	LicenseNo     string `json:"license_no"`
	RenewCardCode string `json:"renew_card_code"`
	BindMode      string `json:"bind_mode"`
	BindValue     string `json:"bind_value"`
}

type UnbindRequest struct {
	ClientInstance
	LicenseNo string `json:"license_no"`
	BindMode  string `json:"bind_mode"`
	BindValue string `json:"bind_value"`
	Reason    string `json:"reason,omitempty"`
}

type LicenseResponse struct {
	BindMode           string         `json:"bind_mode,omitempty"`
	BindValue          string         `json:"bind_value,omitempty"`
	AuthorizationTerms map[string]any `json:"authorization_terms,omitempty"`
	LicenseMode        string         `json:"license_mode"`
	CanUse             bool           `json:"can_use"`
	CanUpdate          bool           `json:"can_update"`
	UsageExpiredAt     *time.Time     `json:"usage_expired_at"`
	UpdatesExpiredAt   *time.Time     `json:"updates_expired_at"`
	LicenseNo          string         `json:"license_no"`
	Status             string         `json:"status"`
	ExpiredAt          *time.Time     `json:"expired_at,omitempty"`
	GraceUntil         *time.Time     `json:"grace_until,omitempty"`
	LicenseToken       string         `json:"license_token,omitempty"`
	OfflineToken       string         `json:"offline_token,omitempty"`
	ServerTime         time.Time      `json:"server_time"`
}

type HeartbeatResponse struct {
	Accepted   bool      `json:"accepted"`
	ServerTime time.Time `json:"server_time"`
}

type UnbindResponse struct {
	Unbound    bool      `json:"unbound"`
	LicenseNo  string    `json:"license_no"`
	ServerTime time.Time `json:"server_time"`
}

// ClientInstance contains only this installation's credential, never AppSecret.
type ClientInstance struct {
	SessionID      string            `json:"session_id,omitempty"`
	InstanceID     string            `json:"instance_id,omitempty"`
	InstanceSecret string            `json:"instance_secret,omitempty"`
	DeviceID       string            `json:"device_id,omitempty"`
	SeatID         string            `json:"seat_id,omitempty"`
	Bindings       map[string]string `json:"bindings,omitempty"`
	Target         *ReleaseTarget    `json:"target,omitempty"`
	CurrentVersion string            `json:"current_version,omitempty"`
}
type ReleaseTarget struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Runtime     string `json:"runtime"`
	Channel     string `json:"channel"`
	Kind        string `json:"kind"`
	MinVersion  string `json:"min_version,omitempty"`
	MaxVersion  string `json:"max_version,omitempty"`
	ImageTag    string `json:"image_tag,omitempty"`
	ImageDigest string `json:"image_digest,omitempty"`
}
type SeatRequest struct {
	VerifyRequest
	SessionID  string `json:"session_id"`
	BorrowDays int    `json:"borrow_days"`
}
type SeatLease struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instance_id"`
	SessionID  string    `json:"session_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	Borrowed   bool      `json:"borrowed"`
	Token      string    `json:"token"`
}
type ClaimTicket struct {
	Token     string    `json:"claim_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

package ynlicense

import "time"

type ActivateRequest struct {
	CardCode      string `json:"card_code"`
	BindMode      string `json:"bind_mode"`
	BindValue     string `json:"bind_value"`
	DeviceName    string `json:"device_name,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
}

type VerifyRequest struct {
	LicenseNo    string `json:"license_no"`
	BindMode     string `json:"bind_mode"`
	BindValue    string `json:"bind_value"`
	LicenseToken string `json:"license_token,omitempty"`
}

type HeartbeatRequest struct {
	LicenseNo string `json:"license_no"`
	BindMode  string `json:"bind_mode"`
	BindValue string `json:"bind_value"`
}

type RenewRequest struct {
	LicenseNo     string `json:"license_no"`
	RenewCardCode string `json:"renew_card_code"`
	BindMode      string `json:"bind_mode"`
	BindValue     string `json:"bind_value"`
}

type UnbindRequest struct {
	LicenseNo string `json:"license_no"`
	BindMode  string `json:"bind_mode"`
	BindValue string `json:"bind_value"`
	Reason    string `json:"reason,omitempty"`
}

type LicenseResponse struct {
	LicenseNo    string     `json:"license_no"`
	Status       string     `json:"status"`
	ExpiredAt    *time.Time `json:"expired_at,omitempty"`
	GraceUntil   *time.Time `json:"grace_until,omitempty"`
	LicenseToken string     `json:"license_token,omitempty"`
	OfflineToken string     `json:"offline_token,omitempty"`
	ServerTime   time.Time  `json:"server_time"`
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

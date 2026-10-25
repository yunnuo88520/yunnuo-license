package domain

import "time"

type EmailSettings struct {
	Enabled         int       `json:"enabled"`
	SMTPHost        string    `json:"smtp_host"`
	SMTPPort        int       `json:"smtp_port"`
	SMTPUsername    string    `json:"smtp_username"`
	SMTPPasswordEnc string    `json:"-"`
	FromAddress     string    `json:"from_address"`
	FromName        string    `json:"from_name"`
	Encryption      string    `json:"encryption"`
	SiteURL         string    `json:"site_url"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type EmailLog struct {
	ID        string    `json:"id"`
	ToAddress string    `json:"to_address"`
	Subject   string    `json:"subject"`
	Kind      string    `json:"kind"`
	AgentID   string    `json:"agent_id"`
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
}

// 邮件通知的业务事件类型。
const (
	EmailKindTest     = "test"
	EmailKindManual   = "manual"
	EmailKindBalance  = "balance_adjust"
	EmailKindQuota    = "quota_grant"
	EmailKindAccount  = "account_created"
	EmailKindStatus   = "status_change"
	EmailKindRecovery = "recovery_code"
	EmailKindExpiry   = "license_expiry"
)

// 加密方式取值。
const (
	EmailEncryptionNone     = "none"
	EmailEncryptionSTARTTLS = "starttls"
	EmailEncryptionSSL      = "ssl"
)

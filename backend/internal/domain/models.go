package domain

import "time"

const (
	BindNone    = "none"
	BindDevice  = "device"
	BindDomain  = "domain"
	BindIP      = "ip"
	BindAccount = "account"

	ConflictReject     = "reject"
	ConflictKickOldest = "kick_oldest"

	ProductActive   = "active"
	ProductDisabled = "disabled"

	CardUnused             = "unused"
	CardActivated          = "activated"
	CardConsumedForRenewal = "consumed_for_renewal"
	CardVoided             = "voided"

	LicenseActive  = "active"
	LicenseExpired = "expired"
	LicenseRevoked = "revoked"

	BindingActive  = "active"
	BindingKicked  = "kicked"
	BindingUnbound = "unbound"
	BindingRevoked = "revoked"

	AgentActive    = "active"
	AgentSuspended = "suspended"
	AgentDisabled  = "disabled"

	AgentUserActive   = "active"
	AgentUserDisabled = "disabled"

	AgentRoleOwner    = "agent_owner"
	AgentRoleManager  = "agent_manager"
	AgentRoleStaff    = "agent_staff"
	AgentRoleReadonly = "agent_readonly"

	AdminUserActive   = "active"
	AdminUserDisabled = "disabled"

	AdminRoleSuperAdmin = "super_admin"
	AdminRoleAdmin      = "admin"
	AdminRoleOperator   = "operator"
	AdminRoleAuditor    = "auditor"

	AgentPolicyActive   = "active"
	AgentPolicyDisabled = "disabled"

	SettlementPrepaid = "prepaid_quota"
	SettlementManual  = "manual"

	QuotaGrant         = "grant"
	QuotaRevoke        = "revoke"
	QuotaGenerateCards = "generate_cards"
)

type Product struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Code                 string    `json:"code"`
	AppKey               string    `json:"app_key"`
	PublicKeyPEM         string    `json:"public_key_pem"`
	PrivateKeyEncrypted  string    `json:"-"`
	KeyVersion           int       `json:"key_version"`
	BindMode             string    `json:"bind_mode"`
	MaxBindCount         int       `json:"max_bind_count"`
	BindConflictStrategy string    `json:"bind_conflict_strategy"`
	OfflineMode          string    `json:"offline_mode"`
	OfflineGraceDays     int       `json:"offline_grace_days"`
	ExpireGraceDays      int       `json:"expire_grace_days"`
	UnbindLimit          int       `json:"unbind_limit"`
	UnbindCooldownHours  int       `json:"unbind_cooldown_hours"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type ProductKey struct {
	ProductID    string    `json:"product_id"`
	KeyVersion   int       `json:"key_version"`
	PublicKeyPEM string    `json:"public_key_pem"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Card struct {
	ID                 string     `json:"id"`
	ProductID          string     `json:"product_id"`
	BatchID            string     `json:"batch_id"`
	AgentID            string     `json:"agent_id,omitempty"`
	CodeHash           string     `json:"-"`
	CodeEncrypted      string     `json:"-"`
	CodePrefix         string     `json:"code_prefix"`
	DurationDays       int        `json:"duration_days"`
	IsPermanent        bool       `json:"is_permanent"`
	Status             string     `json:"status"`
	OrderNo            string     `json:"-"`
	ActivatedLicenseID string     `json:"activated_license_id,omitempty"`
	ActivatedAt        *time.Time `json:"activated_at,omitempty"`
	ConsumedAt         *time.Time `json:"consumed_at,omitempty"`
	VoidedAt           *time.Time `json:"voided_at,omitempty"`
	VoidReason         string     `json:"void_reason,omitempty"`
	CreatedBy          string     `json:"created_by,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type CardBatch struct {
	ID           string    `json:"id"`
	ProductID    string    `json:"product_id"`
	AgentID      string    `json:"agent_id,omitempty"`
	Name         string    `json:"name"`
	Quantity     int       `json:"quantity"`
	DurationDays int       `json:"duration_days"`
	IsPermanent  bool      `json:"is_permanent"`
	Source       string    `json:"source"`
	Status       string    `json:"status"`
	ExportCount  int       `json:"export_count"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type License struct {
	ID                  string     `json:"id"`
	LicenseNo           string     `json:"license_no"`
	ProductID           string     `json:"product_id"`
	CardID              string     `json:"card_id"`
	AgentID             string     `json:"agent_id,omitempty"`
	Status              string     `json:"status"`
	IssuedAt            time.Time  `json:"issued_at"`
	ActivatedAt         time.Time  `json:"activated_at"`
	ExpiredAt           *time.Time `json:"expired_at,omitempty"`
	LastVerifyAt        *time.Time `json:"last_verify_at,omitempty"`
	LastHeartbeatAt     *time.Time `json:"last_heartbeat_at,omitempty"`
	OfflineTokenVersion int        `json:"offline_token_version"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type OfflineLicense struct {
	ID                   string     `json:"id"`
	LicenseNo            string     `json:"license_no"`
	ProductID            string     `json:"product_id"`
	Label                string     `json:"label,omitempty"`
	MachineCodeHash      string     `json:"-"`
	MachineCodeEncrypted string     `json:"-"`
	MachineCodeMasked    string     `json:"machine_code_masked"`
	SignedTokenEncrypted string     `json:"-"`
	TokenVersion         int        `json:"token_version"`
	Status               string     `json:"status"`
	IssuedAt             time.Time  `json:"issued_at"`
	ExpiredAt            *time.Time `json:"expired_at,omitempty"`
	RevokedAt            *time.Time `json:"revoked_at,omitempty"`
	RevokedReason        string     `json:"revoked_reason,omitempty"`
	CreatedBy            string     `json:"created_by,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type Binding struct {
	ID                 string     `json:"id"`
	LicenseID          string     `json:"license_id"`
	ProductID          string     `json:"product_id"`
	BindMode           string     `json:"bind_mode"`
	BindValueHash      string     `json:"-"`
	BindValueEncrypted string     `json:"-"`
	DisplayName        string     `json:"display_name,omitempty"`
	Status             string     `json:"status"`
	FirstSeenIP        string     `json:"first_seen_ip,omitempty"`
	LastSeenIP         string     `json:"last_seen_ip,omitempty"`
	UserAgent          string     `json:"user_agent,omitempty"`
	LastHeartbeatAt    *time.Time `json:"last_heartbeat_at,omitempty"`
	ActivatedAt        time.Time  `json:"activated_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

const (
	RiskBlockIP        = "ip"
	RiskBlockDevice    = "device"
	RiskStatusActive   = "active"
	RiskStatusDisabled = "disabled"
	RiskAlertOpen      = "open"
	RiskAlertResolved  = "resolved"
)

type RiskBlock struct {
	ID          string    `json:"id"`
	ProductID   string    `json:"product_id,omitempty"`
	Kind        string    `json:"kind"`
	ValueHash   string    `json:"-"`
	ValueMasked string    `json:"value_masked"`
	Reason      string    `json:"reason"`
	Status      string    `json:"status"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RiskAlert struct {
	ID              string     `json:"id"`
	ProductID       string     `json:"product_id"`
	LicenseID       string     `json:"license_id,omitempty"`
	BindingID       string     `json:"binding_id,omitempty"`
	AlertType       string     `json:"alert_type"`
	Severity        string     `json:"severity"`
	Status          string     `json:"status"`
	SubjectKind     string     `json:"subject_kind"`
	SubjectHash     string     `json:"-"`
	SubjectMasked   string     `json:"subject_masked"`
	Detail          string     `json:"detail"`
	OccurrenceCount int        `json:"occurrence_count"`
	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy      string     `json:"resolved_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type RiskSummary struct {
	ActiveBlocks   int `json:"active_blocks"`
	OpenAlerts     int `json:"open_alerts"`
	CriticalAlerts int `json:"critical_alerts"`
	Alerts24Hours  int `json:"alerts_24h"`
}

type AuditLog struct {
	ID        string    `json:"id"`
	ActorType string    `json:"actor_type"`
	ActorID   string    `json:"actor_id,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
	ProductID string    `json:"product_id,omitempty"`
	LicenseID string    `json:"license_id,omitempty"`
	CardID    string    `json:"card_id,omitempty"`
	BindingID string    `json:"binding_id,omitempty"`
	Action    string    `json:"action"`
	ClientIP  string    `json:"client_ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	Result    string    `json:"result"`
	ErrorCode string    `json:"error_code,omitempty"`
	ExtraJSON string    `json:"extra_json,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Agent struct {
	ID                  string     `json:"id"`
	AgentNo             string     `json:"agent_no"`
	LoginCode           string     `json:"login_code"`
	ParentAgentID       string     `json:"parent_agent_id,omitempty"`
	Name                string     `json:"name"`
	ContactName         string     `json:"contact_name,omitempty"`
	Phone               string     `json:"phone,omitempty"`
	Email               string     `json:"email,omitempty"`
	Level               int        `json:"level"`
	Status              string     `json:"status"`
	SettlementMode      string     `json:"-"`
	DefaultDiscountRate float64    `json:"-"`
	CreditLimit         int        `json:"-"`
	Remark              string     `json:"remark,omitempty"`
	CreatedBy           string     `json:"created_by,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	DisabledAt          *time.Time `json:"disabled_at,omitempty"`
}

type AgentUser struct {
	ID           string     `json:"id"`
	AgentID      string     `json:"agent_id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	DisplayName  string     `json:"display_name,omitempty"`
	Phone        string     `json:"phone,omitempty"`
	Email        string     `json:"email,omitempty"`
	Role         string     `json:"role"`
	Status       string     `json:"status"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type AdminUser struct {
	ID             string     `json:"id"`
	Username       string     `json:"username"`
	PasswordHash   string     `json:"-"`
	DisplayName    string     `json:"display_name,omitempty"`
	Role           string     `json:"role"`
	Status         string     `json:"status"`
	SessionVersion int        `json:"-"`
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type AgentProductPolicy struct {
	ID                  string    `json:"id"`
	AgentID             string    `json:"agent_id"`
	ProductID           string    `json:"product_id"`
	CanGenerate         bool      `json:"can_generate"`
	CanExportPlainCode  bool      `json:"can_export_plain_code"`
	AllowedDurationDays []int     `json:"allowed_duration_days"`
	AllowPermanent      bool      `json:"allow_permanent"`
	MaxBatchQuantity    int       `json:"max_batch_quantity"`
	DiscountRate        float64   `json:"-"`
	SettlementPrice     int       `json:"-"`
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type AgentQuotaLedger struct {
	ID             string    `json:"id"`
	AgentID        string    `json:"agent_id"`
	ProductID      string    `json:"product_id"`
	DurationDays   int       `json:"duration_days"`
	IsPermanent    bool      `json:"is_permanent"`
	ChangeType     string    `json:"change_type"`
	ChangeQuantity int       `json:"change_quantity"`
	BalanceAfter   int       `json:"balance_after"`
	RelatedBatchID string    `json:"related_batch_id,omitempty"`
	RelatedCardID  string    `json:"related_card_id,omitempty"`
	OperatorType   string    `json:"operator_type"`
	OperatorID     string    `json:"operator_id,omitempty"`
	Remark         string    `json:"remark,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type AgentQuotaSummary struct {
	AgentID      string `json:"agent_id"`
	ProductID    string `json:"product_id"`
	DurationDays int    `json:"duration_days"`
	IsPermanent  bool   `json:"is_permanent"`
	Balance      int    `json:"balance"`
}

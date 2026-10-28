// Package licensing defines the application-scoped authorization contract.
// Transport adapters and SQL implementations depend on this package; it does
// not depend on database/sql or the legacy product authorization service.
package licensing

import (
	"context"
	"errors"
	"time"
)

var (
	ErrScopeRequired = errors.New("licensing: verified application scope required")
	ErrScopeMismatch = errors.New("licensing: application scope mismatch")
	ErrReadOnly      = errors.New("licensing: scope is read only")
	ErrNotFound      = errors.New("licensing: record not found in this application")
	ErrAuditRequired = errors.New("licensing: changes require a transaction audit event")
	ErrConflict      = errors.New("licensing: conflicting or replayed operation")
)

type Actor struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Role      string `json:"role"`
	RequestID string `json:"request_id,omitempty"`
}
type Scope struct {
	applicationID string
	actor         Actor
	writable      bool
}
type scopeKey struct{}

// BindScope is called only by the authorization boundary, after checking the
// actor's application grant. Store operations never accept an optional scope.
func BindScope(ctx context.Context, applicationID string, actor Actor, writable bool) (context.Context, error) {
	if applicationID == "" || actor.Kind == "" {
		return nil, ErrScopeRequired
	}
	if old, ok := ctx.Value(scopeKey{}).(Scope); ok && old.applicationID != applicationID {
		return nil, ErrScopeMismatch
	}
	return context.WithValue(ctx, scopeKey{}, Scope{applicationID, actor, writable}), nil
}
func ScopeFrom(ctx context.Context) (Scope, error) {
	v, ok := ctx.Value(scopeKey{}).(Scope)
	if !ok || v.applicationID == "" {
		return Scope{}, ErrScopeRequired
	}
	return v, nil
}
func (s Scope) ApplicationID() string { return s.applicationID }
func (s Scope) Actor() Actor          { return s.actor }
func (s Scope) Writable() bool        { return s.writable }

type Record struct {
	ApplicationID string    `json:"application_id"`
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (r Record) Identity() Record { return r }

type Entity interface{ Identity() Record }

type VerifyPolicy struct {
	IPSource                 string `json:"ip_source"`
	JWTTTLSeconds            int    `json:"jwt_ttl_seconds"`
	VerifyIntervalSeconds    int    `json:"verify_interval_seconds"`
	NetworkGraceSeconds      int    `json:"network_grace_seconds"`
	SubscriptionGraceSeconds int    `json:"subscription_grace_seconds"`
	SeatLeaseSeconds         int    `json:"seat_lease_seconds"`
	MaxBorrowSeconds         int    `json:"max_borrow_seconds"`
	KeyOverlapSeconds        int    `json:"key_overlap_seconds"`
	DomainRuleMode           string `json:"domain_rule_mode"`
	IPRuleMode               string `json:"ip_rule_mode"`
}
type Application struct {
	BlockPolicy        BlockPolicy `json:"block_policy"`
	Group              string      `json:"group"`
	AllowOwnerTransfer bool        `json:"allow_owner_transfer"`
	Record
	Name               string       `json:"name"`
	DeveloperID        string       `json:"developer_id"`
	Status             string       `json:"status"`
	AllowedModels      []string     `json:"allowed_models"`
	AllowedBindings    []string     `json:"allowed_bindings"`
	Forms              []string     `json:"forms"`
	VerifyPolicy       VerifyPolicy `json:"verify_policy"`
	SecretHash         string       `json:"-"`
	SecretVersion      int          `json:"secret_version"`
	CurrentKID         string       `json:"current_kid"`
	PublicQueryEnabled bool         `json:"public_query_enabled"`
	UpdatesEnabled     bool         `json:"updates_enabled"`
}
type SigningKey struct {
	MaxIssuedUntil *time.Time `json:"max_issued_until"`
	Record
	KID           string     `json:"kid"`
	Version       int        `json:"version"`
	Algorithm     string     `json:"algorithm"`
	PublicPEM     string     `json:"public_key_pem"`
	PrivateCipher string     `json:"-"`
	Status        string     `json:"status"`
	AcceptUntil   *time.Time `json:"accept_until"`
}
type Terms struct {
	RenewalTargetPlanIDs  []string `json:"renewal_target_plan_ids,omitempty"`
	MaxBindingGroups      int      `json:"max_binding_groups"`
	StartsOn              string   `json:"starts_on"`
	Model                 string   `json:"model"`
	BindingTypes          []string `json:"binding_types"`
	Forms                 []string `json:"forms"`
	Perpetual             bool     `json:"perpetual"`
	DurationDays          int      `json:"duration_days"`
	UpdateDurationDays    int      `json:"update_duration_days"`
	MaxActivations        int      `json:"max_activations"`
	MaxSeats              int      `json:"max_seats"`
	MaxDomainAliases      int      `json:"max_domain_aliases"`
	UsageQuota            int64    `json:"usage_quota"`
	UsageUnit             string   `json:"usage_unit"`
	UsagePeriod           string   `json:"usage_period"`
	RebindLimit           int      `json:"rebind_limit"`
	RebindCooldownSeconds int      `json:"rebind_cooldown_seconds"`
	Features              []string `json:"features"`
}
type Plan struct {
	Record
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	Terms          Terms  `json:"terms"`
	PriceCents     int64  `json:"price_cents"`
	AgentCostCents int64  `json:"agent_cost_cents"`
	Currency       string `json:"currency"`
}
type License struct {
	BaseDurationDays int        `json:"base_duration_days"`
	BaseUpdateDays   int        `json:"base_update_days"`
	BaseExpiresAt    *time.Time `json:"base_expires_at"`
	BaseUpdatesUntil *time.Time `json:"base_updates_until"`

	UpdatesBlocked bool       `json:"updates_blocked"`
	RenewalOnly    bool       `json:"renewal_only"`
	ActivatedAt    *time.Time `json:"activated_at"`
	Record
	PlanID       string     `json:"plan_id"`
	CustomerRef  string     `json:"customer_ref"`
	OwnerUserID  string     `json:"owner_user_id"`
	OrderID      string     `json:"order_id"`
	Status       string     `json:"status"`
	Terms        Terms      `json:"terms"`
	BindingType  string     `json:"binding_type"`
	BindingHash  string     `json:"binding_hash,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at"`
	UpdatesUntil *time.Time `json:"updates_until"`
	UsedUnits    int64      `json:"used_units"`
	UsagePeriod  string     `json:"usage_period"`
	RebindsUsed  int        `json:"rebinds_used"`
	LastRebindAt *time.Time `json:"last_rebind_at"`
	Reason       string     `json:"reason,omitempty"`
}
type Activation struct {
	DeactivationReason string `json:"deactivation_reason,omitempty"`
	BindingGroupID     string `json:"binding_group_id"`
	Record
	LicenseID         string     `json:"license_id"`
	InstallationID    string     `json:"installation_id"`
	PublicKey         string     `json:"public_key"`
	PublicKeyHash     string     `json:"public_key_hash"`
	SessionID         string     `json:"session_id"`
	BindingType       string     `json:"binding_type"`
	BindingHash       string     `json:"binding_hash"`
	ClientBindingHash string     `json:"client_binding_hash"`
	Label             string     `json:"label"`
	LeaseExpiresAt    *time.Time `json:"lease_expires_at"`
	Borrowed          bool       `json:"borrowed"`
	LastVerifiedAt    time.Time  `json:"last_verified_at"`
	DeactivatedAt     *time.Time `json:"deactivated_at"`
}
type CardKey struct {
	RefundedAt        *time.Time `json:"refunded_at"`
	Source            string     `json:"source,omitempty"`
	RedeemedLicenseID string     `json:"redeemed_license_id,omitempty"`
	Record
	PlanID         string     `json:"plan_id"`
	BatchID        string     `json:"batch_id"`
	KeyHash        string     `json:"-"`
	LastFour       string     `json:"last_four"`
	Purpose        string     `json:"purpose"`
	Status         string     `json:"status"`
	BoundLicenseID string     `json:"bound_license_id"`
	UsedAt         *time.Time `json:"used_at"`
	RedeemUntil    *time.Time `json:"redeem_until"`
}
type BindingRule struct {
	Record
	Kind           string `json:"kind"`
	Pattern        string `json:"pattern"`
	Enabled        bool   `json:"enabled"`
	IncludeApex    bool   `json:"include_apex"`
	SubdomainDepth string `json:"subdomain_depth"`
	LicenseID      string `json:"license_id,omitempty"`
	Label          string `json:"label"`
}
type BindingAnchor struct {
	Record
	LicenseID   string `json:"license_id"`
	GroupID     string `json:"group_id"`
	Kind        string `json:"kind"`
	BindingHash string `json:"binding_hash"`
	ClientHash  string `json:"client_hash"`
	Display     string `json:"display"`
	Status      string `json:"status"`
}
type Invitation struct {
	Record
	LicenseID     string     `json:"license_id"`
	TokenHash     string     `json:"-"`
	Kind          string     `json:"kind"`
	OwnerUserID   string     `json:"owner_user_id"`
	ExpiresAt     time.Time  `json:"expires_at"`
	UsedAt        *time.Time `json:"used_at"`
	PublicKeyHash string     `json:"public_key_hash"`
}
type PendingInstallation struct {
	Record
	LicenseID         string    `json:"license_id"`
	PublicKey         string    `json:"public_key"`
	PublicKeyHash     string    `json:"public_key_hash"`
	BindingType       string    `json:"binding_type"`
	BindingHash       string    `json:"binding_hash"`
	ClientBindingHash string    `json:"client_binding_hash"`
	Display           string    `json:"display"`
	Status            string    `json:"status"`
	ExpiresAt         time.Time `json:"expires_at"`
	ActivationID      string    `json:"activation_id"`
}
type RequestNonce struct {
	Record
	CredentialID string    `json:"credential_id"`
	NonceHash    string    `json:"nonce_hash"`
	BodyHash     string    `json:"body_hash"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type SalesChannel struct {
	Record
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Enabled        bool     `json:"enabled"`
	SecretCipher   string   `json:"-"`
	Version        int      `json:"version"`
	AllowedPlanIDs []string `json:"allowed_plan_ids"`
}
type SalesOrder struct {
	CardDigests []string   `json:"card_sha256,omitempty"`
	Purpose     string     `json:"purpose"`
	RedeemUntil *time.Time `json:"redeem_until"`
	Record
	ExternalID      string   `json:"external_id"`
	ChannelID       string   `json:"channel_id"`
	AgentID         string   `json:"agent_id"`
	PlanID          string   `json:"plan_id"`
	CustomerRef     string   `json:"customer_ref"`
	OwnerUserID     string   `json:"owner_user_id"`
	Quantity        int      `json:"quantity"`
	AmountCents     int64    `json:"amount_cents"`
	CostCents       int64    `json:"cost_cents"`
	Currency        string   `json:"currency"`
	Status          string   `json:"status"`
	Terms           Terms    `json:"terms"`
	DistributeCards bool     `json:"distribute_cards"`
	LicenseIDs      []string `json:"license_ids"`
	Reason          string   `json:"reason,omitempty"`
}
type WebhookReceipt struct {
	Record
	ChannelID  string `json:"channel_id"`
	ExternalID string `json:"external_id"`
	BodyHash   string `json:"body_hash"`
	OrderID    string `json:"order_id"`
	Result     string `json:"result"`
}
type UsageReceipt struct {
	Record
	LicenseID      string `json:"license_id"`
	ActivationID   string `json:"activation_id"`
	IdempotencyKey string `json:"idempotency_key"`
	RequestHash    string `json:"request_hash"`
	Units          int64  `json:"units"`
	Period         string `json:"period"`
	Remaining      int64  `json:"remaining"`
}
type AgentGrant struct {
	Record
	AgentID           string `json:"agent_id"`
	PlanID            string `json:"plan_id"`
	Enabled           bool   `json:"enabled"`
	OverrideCostCents *int64 `json:"override_cost_cents"`
	MaxBatchQuantity  int    `json:"max_batch_quantity"`
}
type MoneyEvent struct {
	RequestHash string `json:"request_hash"`
	Record
	AgentID      string `json:"agent_id"`
	OrderID      string `json:"order_id"`
	RequestID    string `json:"request_id"`
	AmountCents  int64  `json:"amount_cents"`
	BalanceAfter int64  `json:"balance_after"`
	Kind         string `json:"kind"`
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
type Release struct {
	TargetKey string `json:"target_key"`
	Record
	Version     string        `json:"version"`
	Target      ReleaseTarget `json:"target"`
	Status      string        `json:"status"`
	Notes       string        `json:"notes"`
	Filename    string        `json:"filename"`
	StoragePath string        `json:"-"`
	Size        int64         `json:"size"`
	SHA256      string        `json:"sha256"`
}
type AuditEvent struct {
	Record
	Sequence     int64          `json:"sequence"`
	EventType    string         `json:"event_type"`
	EntityID     string         `json:"entity_id"`
	Actor        Actor          `json:"actor"`
	Result       string         `json:"result"`
	ReasonCode   string         `json:"reason_code,omitempty"`
	Payload      map[string]any `json:"payload"`
	PreviousHash string         `json:"previous_hash"`
	EventHash    string         `json:"event_hash"`
}
type AuditHead struct {
	Sequence int64
	Hash     string
}
type AuditSigner func(AuditEvent) (string, error)

type AgentAccount struct {
	AgentID      string `json:"agent_id"`
	Status       string `json:"status"`
	BalanceCents int64  `json:"balance_cents"`
	TierID       string `json:"tier_id"`
	CostRateBPS  int64  `json:"cost_rate_bps"`
}

type ReleaseCapacity struct {
	TotalBytes, DeveloperBytes, ApplicationBytes int64
	Versions                                     int
}
type StorageLimits struct {
	TotalBytes, DeveloperBytes, ApplicationBytes int64
	Versions                                     int
}

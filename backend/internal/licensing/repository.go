package licensing

import (
	"context"
	"time"
)

// Filter contains business fields only. Application scope is deliberately
// absent: the repository injects it, including when every filter is empty.
type Filter struct {
	BoundLicenseID, RedeemedLicenseID                                                          string
	ExactSessionID                                                                             *string
	Borrowed                                                                                   *bool
	ExcludeRenewals                                                                            bool
	ExpiresAfter                                                                               *time.Time
	ActiveAt                                                                                   *time.Time
	Before                                                                                     *time.Time
	From                                                                                       *time.Time
	LeaseOnly, OnlyGlobal, Ascending                                                           bool
	RequestID                                                                                  string
	Result, ReasonCode                                                                         string
	ID, Status, PlanID, LicenseID, CustomerRef, OwnerUserID, BatchID, KeyHash                  string
	BindingType, BindingHash, PublicKeyHash, SessionID, Kind, GroupID                          string
	ChannelID, ExternalID, AgentID, NonceHash, CredentialID, IdempotencyKey, Period, EventType string
	Limit, Offset                                                                              int
}
type ReadTable[T Entity] interface {
	Get(context.Context, string) (T, error)
	Find(context.Context, Filter) ([]T, error)
	Count(context.Context, Filter) (int64, error)
}
type WriteTable[T Entity] interface {
	ReadTable[T]
	Insert(context.Context, T) error
	Save(context.Context, T) error
	Delete(context.Context, string) error
	Lock(context.Context, string) error
}
type View interface {
	AgentAccount(context.Context, string) (AgentAccount, error)
	Application(context.Context) (Application, error)
	Keys() ReadTable[SigningKey]
	Plans() ReadTable[Plan]
	Licenses() ReadTable[License]
	Activations() ReadTable[Activation]
	Cards() ReadTable[CardKey]
	Rules() ReadTable[BindingRule]
	Anchors() ReadTable[BindingAnchor]
	Invitations() ReadTable[Invitation]
	PendingInstallations() ReadTable[PendingInstallation]
	Nonces() ReadTable[RequestNonce]
	Channels() ReadTable[SalesChannel]
	Orders() ReadTable[SalesOrder]
	Webhooks() ReadTable[WebhookReceipt]
	Usage() ReadTable[UsageReceipt]
	AgentGrants() ReadTable[AgentGrant]
	MoneyEvents() ReadTable[MoneyEvent]
	Releases() ReadTable[Release]
	Audit() ReadTable[AuditEvent]
	AuditHead(context.Context) (AuditHead, error)
	Diagnostics(context.Context, time.Time) (Diagnostics, error)
}
type Transaction interface {
	CountBindingGroups(context.Context, string) (int64, error)
	RetireBindings(context.Context, string, time.Time) error
	DeactivateAll(context.Context, string, time.Time) error
	InvalidateInvitations(context.Context, string, time.Time) error
	ReleaseCapacity(context.Context) (ReleaseCapacity, error)
	AgentAccount(context.Context, string) (AgentAccount, error)
	DebitAgent(context.Context, string, int64) (int64, error)
	CreditAgent(context.Context, string, int64) (int64, error)
	LockApplication(context.Context) error
	Application(context.Context) (Application, error)
	SaveApplication(context.Context, Application) error
	Keys() WriteTable[SigningKey]
	Plans() WriteTable[Plan]
	Licenses() WriteTable[License]
	Activations() WriteTable[Activation]
	Cards() WriteTable[CardKey]
	Rules() WriteTable[BindingRule]
	Anchors() WriteTable[BindingAnchor]
	Invitations() WriteTable[Invitation]
	PendingInstallations() WriteTable[PendingInstallation]
	Nonces() WriteTable[RequestNonce]
	Channels() WriteTable[SalesChannel]
	Orders() WriteTable[SalesOrder]
	Webhooks() WriteTable[WebhookReceipt]
	Usage() WriteTable[UsageReceipt]
	AgentGrants() WriteTable[AgentGrant]
	MoneyEvents() WriteTable[MoneyEvent]
	Releases() WriteTable[Release]
	AppendAudit(context.Context, AuditEvent, AuditSigner) error
}
type Repository interface {
	View(context.Context, func(View) error) error
	Update(context.Context, func(Transaction) error) error
}

// Registry is the explicitly privileged application directory, not a business
// query escape hatch. Tenant business objects are unavailable through it.
type Registry interface {
	GetApplication(context.Context, string) (Application, error)
	ListApplications(context.Context, string) ([]Application, error)
	CreateApplication(context.Context, Application, SigningKey, AuditEvent, AuditSigner) error
	Scoped(context.Context) (Repository, error)
}

type Diagnostics struct {
	LicenseTotal      int64            `json:"license_total"`
	ActiveActivations int64            `json:"active_activations"`
	ExpiringLicenses  int64            `json:"expiring_licenses"`
	VerifyFailures    map[string]int64 `json:"verify_failures"`
}

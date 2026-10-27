package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// AuthorizationTerms is copied from a package to stock cards and then licenses.
// A product default can change without rewriting sold or unactivated grants.
type AuthorizationTerms struct {
	CardPurpose         string   `json:"card_purpose"`
	Version             int      `json:"version"`
	Forms               []string `json:"forms"`
	BindMode            string   `json:"bind_mode"`
	BindingKinds        []string `json:"binding_kinds,omitempty"`
	IncludeApex         bool     `json:"include_apex"`
	SubdomainDepth      string   `json:"subdomain_depth"`
	MaxBindings         int      `json:"max_bindings"`
	RequireInstance     bool     `json:"require_instance"`
	MaxDeployments      int      `json:"max_deployments"`
	MaxDevices          int      `json:"max_devices"`
	MaxConcurrentSeats  int      `json:"max_concurrent_seats"`
	SeatLeaseSeconds    int      `json:"seat_lease_seconds"`
	OfflineGraceDays    int      `json:"offline_grace_days"`
	LicenseMode         string   `json:"license_mode"`
	UpdateDurationDays  int      `json:"update_duration_days"`
	ExpireGraceDays     int      `json:"expire_grace_days"`
	UnbindLimit         int      `json:"unbind_limit"`
	UnbindCooldownHours int      `json:"unbind_cooldown_hours"`
	Features            []string `json:"features"`
}

// Apply keeps mutable product status, ownership, keys and presentation current.
func (t AuthorizationTerms) Apply(p Product) Product {
	p.BindMode, p.MaxBindCount = t.BindMode, t.MaxBindings
	p.LicenseMode, p.UpdateDurationDays = t.LicenseMode, t.UpdateDurationDays
	p.ExpireGraceDays, p.OfflineGraceDays = t.ExpireGraceDays, t.OfflineGraceDays
	p.UnbindLimit, p.UnbindCooldownHours = t.UnbindLimit, t.UnbindCooldownHours
	p.AuthorizationTerms = &t
	return p
}

func LegacyTerms(p Product) AuthorizationTerms {
	if p.AuthorizationTerms != nil {
		return *p.AuthorizationTerms
	}
	return AuthorizationTerms{CardPurpose: "license", Version: 1, Forms: []string{"web", "gui", "service"}, BindMode: p.BindMode,
		MaxBindings: p.MaxBindCount, MaxDeployments: 1, SeatLeaseSeconds: 120,
		OfflineGraceDays: p.OfflineGraceDays, LicenseMode: p.LicenseMode,
		UpdateDurationDays: p.UpdateDurationDays, ExpireGraceDays: p.ExpireGraceDays,
		UnbindLimit: p.UnbindLimit, UnbindCooldownHours: p.UnbindCooldownHours,
		SubdomainDepth: "one", Features: []string{}}
}

type ClientInstance struct {
	SessionID      string            `json:"session_id,omitempty"`
	Target         *ReleaseTarget    `json:"target,omitempty"`
	CurrentVersion string            `json:"current_version,omitempty"`
	InstanceID     string            `json:"instance_id,omitempty"`
	InstanceSecret string            `json:"instance_secret,omitempty"`
	DeviceID       string            `json:"device_id,omitempty"`
	SeatID         string            `json:"seat_id,omitempty"`
	Bindings       map[string]string `json:"bindings,omitempty"`
}

// Keep the historical version unique key while allowing multiple target files.
// Public versions are stored alongside the signed target, never shown as hashes.
func ReleaseStorageVersion(version string, target *ReleaseTarget) string {
	if target == nil {
		return version
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{version, target.OS, target.Arch, target.Runtime, target.Channel, target.Kind}, "\x00")))
	return "@" + hex.EncodeToString(sum[:15])
}

type Installation struct {
	LicenseID  string    `json:"-"`
	ID         string    `json:"id"`
	BindingID  string    `json:"binding_id"`
	SecretHash string    `json:"-"`
	DeviceHash string    `json:"-"`
	Label      string    `json:"label"`
	Status     string    `json:"status"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type SeatLease struct {
	ID         string    `json:"id"`
	LicenseID  string    `json:"-"`
	InstanceID string    `json:"instance_id"`
	SessionID  string    `json:"session_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	Borrowed   bool      `json:"borrowed"`
	Token      string    `json:"token,omitempty"`
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

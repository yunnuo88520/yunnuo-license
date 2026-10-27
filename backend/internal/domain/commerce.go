package domain

import "time"

// Costs and balances use integer minor currency units (CNY cents).
type ProductGroup struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ProductIDs []string `json:"product_ids"`
}
type CardPackage struct {
	AuthorizationTerms *AuthorizationTerms `json:"authorization_terms,omitempty"`
	ID                 string              `json:"id"`
	ProductID          string              `json:"product_id"`
	Name               string              `json:"name"`
	DurationDays       int                 `json:"duration_days"`
	IsPermanent        bool                `json:"is_permanent"`
	BaseCost           int64               `json:"base_cost"`
	UseQuota           bool                `json:"use_quota"`
	Enabled            bool                `json:"enabled"`
}
type AgentTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CostRateBPS int64  `json:"cost_rate_bps"`
	SortOrder   int    `json:"sort_order"`
	AgentCount  int    `json:"agent_count"`
}
type PackageGrant struct {
	AgentID      string `json:"agent_id"`
	PackageID    string `json:"package_id"`
	Enabled      bool   `json:"enabled"`
	OverrideCost *int64 `json:"override_cost"`
}
type PackageQuote struct {
	Package     CardPackage `json:"package"`
	UnitCost    int64       `json:"unit_cost"`
	PriceSource string      `json:"price_source"`
}
type AgentAccount struct {
	AgentID string `json:"agent_id"`
	TierID  string `json:"tier_id"`
	Balance int64  `json:"balance"`
}
type MoneyLedger struct {
	ID           string    `json:"id"`
	AgentID      string    `json:"agent_id"`
	RequestID    string    `json:"request_id"`
	Kind         string    `json:"kind"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balance_after"`
	BatchID      string    `json:"batch_id"`
	PackageID    string    `json:"package_id"`
	UnitCost     int64     `json:"unit_cost"`
	Quantity     int       `json:"quantity"`
	Remark       string    `json:"remark"`
	ActorID      string    `json:"actor_id"`
	CreatedAt    time.Time `json:"created_at"`
}

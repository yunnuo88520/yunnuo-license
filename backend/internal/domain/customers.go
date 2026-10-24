package domain

import "time"

type Customer struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	PasswordHash   string    `json:"-"`
	DisplayName    string    `json:"display_name"`
	Status         string    `json:"status"`
	SessionVersion int       `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}

type BindingPolicy struct {
	ProductID          string `json:"product_id"`
	BindMode           string `json:"bind_mode"`
	Allowed            bool   `json:"allowed"`
	Limit              int    `json:"limit"`
	CooldownHours      int    `json:"cooldown_hours"`
	AllowOwnerTransfer bool   `json:"allow_owner_transfer"`
}

type CustomerBinding struct {
	ID     string `json:"id"`
	Mode   string `json:"mode"`
	Value  string `json:"value"`
	Status string `json:"status"`
}

type CustomerLicense struct {
	CanUse       bool              `json:"can_use"`
	CanUpdate    bool              `json:"can_update"`
	License      License           `json:"license"`
	Product      Product           `json:"product"`
	Bindings     []CustomerBinding `json:"bindings"`
	Policy       BindingPolicy     `json:"policy"`
	ChangesUsed  int               `json:"changes_used"`
	LastChangeAt *time.Time        `json:"last_change_at"`
}

type OperationLog struct {
	ObjectID   string    `json:"object_id"`
	ID         string    `json:"id"`
	RequestID  string    `json:"request_id"`
	Category   string    `json:"category"`
	ActorType  string    `json:"actor_type"`
	ActorID    string    `json:"actor_id"`
	Role       string    `json:"role"`
	Method     string    `json:"method"`
	Route      string    `json:"route"`
	ClientIP   string    `json:"client_ip"`
	HTTPStatus int       `json:"http_status"`
	Result     string    `json:"result"`
	ErrorCode  string    `json:"error_code"`
	CreatedAt  time.Time `json:"created_at"`
}

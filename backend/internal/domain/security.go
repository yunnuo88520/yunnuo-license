package domain

import "time"

type SecurityPolicy struct {
	ProductID      string `json:"product_id"`
	Enabled        bool   `json:"enabled"`
	DetectMismatch bool   `json:"detect_mismatch"`
	AcceptReports  bool   `json:"accept_reports"`
	Action         string `json:"action"`
	Message        string `json:"message"`
	SupportURL     string `json:"support_url"`
}
type SecurityCase struct {
	ID           string    `json:"id"`
	ProductID    string    `json:"product_id"`
	LicenseID    string    `json:"-"`
	LicenseNo    string    `json:"license_no"`
	Signal       string    `json:"signal"`
	Subject      string    `json:"subject"`
	Evidence     string    `json:"evidence"`
	Status       string    `json:"status"`
	Action       string    `json:"action"`
	Occurrences  int       `json:"occurrences"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	ReviewReason string    `json:"review_reason"`
	Reviewer     string    `json:"reviewer"`
}
type SecurityAppeal struct {
	ID        string    `json:"id"`
	CaseID    string    `json:"case_id"`
	UserID    string    `json:"user_id"`
	Reason    string    `json:"reason"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

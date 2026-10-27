package domain

import "time"

type ProductUpdateSettings struct {
	ProductID string `json:"product_id"`
	Enabled   bool   `json:"enabled"`
}

type ProductRelease struct {
	Target        *ReleaseTarget `json:"target,omitempty"`
	ID            string         `json:"id"`
	ProductID     string         `json:"product_id"`
	Version       string         `json:"version"`
	Notes         string         `json:"notes"`
	Status        string         `json:"status"`
	Filename      string         `json:"filename"`
	Size          int64          `json:"size"`
	SHA256        string         `json:"sha256"`
	KeyVersion    int            `json:"key_version"`
	ManifestToken string         `json:"manifest_token"`
	CreatedAt     time.Time      `json:"created_at"`
}

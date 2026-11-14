package llmgateway

import (
	"time"
)

// Provider is one configured LLM backend for a workspace.
//
// The API key is stored sealed by the host crypto vault and is never serialized
// to a client: the API exposes HasKey instead. One workspace has at most one
// default provider; a workspace with no provider has no AI, which is the
// fail-closed state (there is no environment fallback).
type Provider struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	OrgID      uint      `gorm:"column:org_id;index:idx_llm_gateway_provider_org;not null" json:"orgId"`
	Name       string    `gorm:"size:128;not null" json:"name"`
	Backend    string    `gorm:"size:32;not null" json:"backend"`
	APIKeyEncr string    `gorm:"type:text" json:"-"`
	BaseURL    string    `gorm:"size:256" json:"baseUrl"`
	Model      string    `gorm:"size:64" json:"model"`
	CheapModel string    `gorm:"size:64" json:"cheapModel"`
	IsDefault  bool      `gorm:"default:false" json:"isDefault"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// TableName fixes the table name so a rename of the Go type never migrates a
// second table.
func (Provider) TableName() string { return "llm_gateway_providers" }

// ModelPrice is the price of one model per one million tokens.
//
// A turn is billed at the price of the model it used. A model without a row
// here is refused rather than billed at zero: a ceiling over a zero price could
// never trip.
type ModelPrice struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     uint      `gorm:"column:org_id;uniqueIndex:idx_llm_gateway_price,priority:1;not null" json:"orgId"`
	Model     string    `gorm:"size:128;uniqueIndex:idx_llm_gateway_price,priority:2;not null" json:"model"`
	Input     float64   `json:"input"`
	Output    float64   `json:"output"`
	Reasoning float64   `json:"reasoning"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName fixes the price table name.
func (ModelPrice) TableName() string { return "llm_gateway_prices" }

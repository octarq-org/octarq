package llmgateway

import (
	"time"
)

type Provider struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	OrgID       uint      `gorm:"column:org_id;index:idx_llm_gateway_provider_org;not null" json:"orgId"`
	Name        string    `gorm:"size:128;not null" json:"name"`
	Backend     string    `gorm:"size:32;not null" json:"backend"`
	VaultItemID uint      `gorm:"column:vault_item_id;default:0" json:"-"`
	BaseURL     string    `gorm:"size:256" json:"baseUrl"`
	Model       string    `gorm:"size:128" json:"model"`
	CheapModel  string    `gorm:"size:128" json:"cheapModel"`
	IsDefault   bool      `gorm:"default:false" json:"isDefault"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (Provider) TableName() string { return "llm_gateway_providers" }

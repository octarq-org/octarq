package models

import "time"

// AgentHarnessSpec is one published version of a workspace's agent harness.
//
// Versions are immutable: publishing a new one archives the previous, and a
// rollback re-publishes an older row. The spec itself is stored as the
// canonical JSON of harness.HarnessSpec, and the fingerprint is the hash of
// that document — two rows with the same fingerprint compile the same prompt.
type AgentHarnessSpec struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// OrgID scopes the row to one workspace. The column is owner_id for
	// consistency with the rest of the tenant-scoped tables.
	OrgID   uint   `gorm:"index:idx_agent_harness_org;not null;column:owner_id" json:"orgId"`
	Name    string `gorm:"size:64;not null" json:"name"`
	Version int    `gorm:"not null" json:"version"`
	// SpecJSON is the canonical spec document.
	SpecJSON string `gorm:"type:text;not null" json:"spec"`
	// Fingerprint is the SHA-256 of the canonical spec document.
	Fingerprint string `gorm:"size:64;index;not null" json:"fingerprint"`
	// Status is "published" for the active version and "archived" for the rest.
	Status      string     `gorm:"size:16;not null;index" json:"status"`
	CreatedBy   uint       `gorm:"not null" json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	PublishedAt *time.Time `json:"publishedAt"`
}

func (AgentHarnessSpec) TableName() string {
	return "agent_harness_specs"
}

// Agent harness spec statuses.
const (
	AgentHarnessStatusPublished = "published"
	AgentHarnessStatusArchived  = "archived"
)

package ontology

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// AppliedSpecTable is the bookkeeping table that records which declaration is
// physically applied. It exists because the physical schema cannot be compared
// back to a declaration: SQLite stores string, text, uuid and json all as TEXT,
// so a type change is invisible from column metadata alone.
//
// The declaration itself is never stored here as a source of truth — the config
// file is. A row means "at the last successful boot this exact document was
// applied", which is what ValidateTransition needs to diff against.
const AppliedSpecTable = "ont_applied_specs"

// AppliedSpec is one bookkeeping row.
type AppliedSpec struct {
	Slug      string    `gorm:"primaryKey;size:64;column:slug"`
	SpecJSON  string    `gorm:"type:text;not null;column:spec_json"`
	Hash      string    `gorm:"size:64;not null;column:hash"`
	AppliedAt time.Time `gorm:"column:applied_at"`
}

// TableName implements gorm.Tabler.
func (AppliedSpec) TableName() string { return AppliedSpecTable }

// Models returns the models the ontology layer owns, for plugin.Models().
func Models() []any { return []any{&AppliedSpec{}} }

// Hash is a stable digest of a declaration. It exists so boot can skip an
// unchanged entity without re-reading columns, and so an operator can compare
// two installations at a glance.
func (s Spec) Hash() (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

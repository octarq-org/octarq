package ontology

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

const AppliedSpecTable = "ont_applied_specs"

type AppliedSpec struct {
	Slug      string    `gorm:"primaryKey;size:64;column:slug"`
	SpecJSON  string    `gorm:"type:text;not null;column:spec_json"`
	Hash      string    `gorm:"size:64;not null;column:hash"`
	AppliedAt time.Time `gorm:"column:applied_at"`
}

func (AppliedSpec) TableName() string { return AppliedSpecTable }

func Models() []any { return []any{&AppliedSpec{}} }

func (s Spec) Hash() (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

package app

import (
	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// settingsStore adapts the settings table to crypto.SecretStore so the envelope
// bootstrap can persist the wrapped DEK alongside other runtime settings.
type settingsStore struct{ db *gorm.DB }

func (s settingsStore) Get(key string) (string, bool) {
	var row models.Setting
	if s.db.First(&row, "key = ?", key).Error != nil {
		return "", false
	}
	return row.Value, true
}

func (s settingsStore) Set(key, val string) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&models.Setting{Key: key, Value: val}).Error
}

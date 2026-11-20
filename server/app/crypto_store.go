package app

import (
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

func (s settingsStore) Delete(key string) error {
	return s.db.Delete(&models.Setting{Key: key}).Error
}

func (s settingsStore) HasEncryptedValues() (bool, error) {
	return models.HasEncryptedValues(s.db)
}

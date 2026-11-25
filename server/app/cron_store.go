package app

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/octarq-org/octarq/server/internal/models"
)

type cronSettingsStore struct {
	db *gorm.DB
}

func (s cronSettingsStore) Get(key string) (string, bool) {
	var row models.Setting
	if err := s.db.First(&row, "key = ?", key).Error; err != nil {
		return "", false
	}
	return row.Value, true
}

func (s cronSettingsStore) Set(key string, value string) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&models.Setting{Key: key, Value: value}).Error
}

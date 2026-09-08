package notification

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DefaultChannels is the fallback channel list when no user preference matches.
var DefaultChannels = []string{"in_app", "email"}

// PreferenceItem represents an event pattern to channels routing mapping.
type PreferenceItem struct {
	EventPattern string   `json:"eventPattern"`
	Channels     []string `json:"channels"`
}

// MatchEventPattern reports whether eventType matches pattern.
// Supports exact matches ("security.login_failed"), wildcard globs ("security.*", "cron.*"),
// and universal wildcard ("*").
func MatchEventPattern(pattern, eventType string) bool {
	pattern = strings.TrimSpace(pattern)
	eventType = strings.TrimSpace(eventType)
	if pattern == "" || eventType == "" {
		return false
	}
	if pattern == "*" || pattern == eventType {
		return true
	}
	// Try standard path.Match
	if matched, err := path.Match(pattern, eventType); err == nil && matched {
		return true
	}
	// Check trailing wildcard like "security.*"
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		if eventType == prefix || strings.HasPrefix(eventType, prefix+".") {
			return true
		}
	}
	// Check prefix wildcard like "*.failed"
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		if eventType == suffix || strings.HasSuffix(eventType, "."+suffix) {
			return true
		}
	}
	return false
}

// ResolveChannels finds the matching channels for a given user and eventType.
// If the user has no matching preferences, DefaultChannels is returned.
func ResolveChannels(ctx context.Context, db *gorm.DB, userID, eventType string) ([]string, error) {
	if db == nil || strings.TrimSpace(userID) == "" {
		return DefaultChannels, nil
	}
	var prefs []models.NotificationPreference
	if err := db.WithContext(ctx).Where("user_id = ?", userID).Find(&prefs).Error; err != nil {
		return DefaultChannels, err
	}
	if len(prefs) == 0 {
		return DefaultChannels, nil
	}

	// Score matches:
	// Exact match: score 1000 + len(pattern)
	// Glob match: score 100 + len(pattern)
	// Universal "*": score 1
	type scoredMatch struct {
		score    int
		channels []string
	}
	var matches []scoredMatch
	for _, p := range prefs {
		if !MatchEventPattern(p.EventPattern, eventType) {
			continue
		}
		var score int
		switch p.EventPattern {
		case eventType:
			score = 1000 + len(p.EventPattern)
		case "*":
			score = 1
		default:
			score = 100 + len(p.EventPattern)
		}
		matches = append(matches, scoredMatch{
			score:    score,
			channels: []string(p.Channels),
		})
	}

	if len(matches) == 0 {
		return DefaultChannels, nil
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	res := matches[0].channels
	if len(res) == 0 {
		return DefaultChannels, nil
	}
	return res, nil
}

// GetPreferences returns all notification preferences configured for a user.
func GetPreferences(ctx context.Context, db *gorm.DB, userID string) ([]models.NotificationPreference, error) {
	if db == nil {
		return nil, errors.New("notification: nil db")
	}
	var prefs []models.NotificationPreference
	err := db.WithContext(ctx).Where("user_id = ?", userID).Order("id ASC").Find(&prefs).Error
	return prefs, err
}

// SavePreferences upserts the specified preference items for a user.
func SavePreferences(ctx context.Context, db *gorm.DB, userID string, items []PreferenceItem) error {
	if db == nil {
		return errors.New("notification: nil db")
	}
	if strings.TrimSpace(userID) == "" {
		return errors.New("notification: empty user ID")
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			pattern := strings.TrimSpace(item.EventPattern)
			if pattern == "" {
				continue
			}
			channels := item.Channels
			if channels == nil {
				channels = []string{}
			}
			pref := models.NotificationPreference{
				UserID:       userID,
				EventPattern: pattern,
				Channels:     models.StringList(channels),
				UpdatedAt:    time.Now(),
			}
			// Upsert based on (user_id, event_pattern)
			err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}, {Name: "event_pattern"}},
				DoUpdates: clause.AssignmentColumns([]string{"channels", "updated_at"}),
			}).Create(&pref).Error
			if err != nil {
				// Fallback if clause.OnConflict unsupported
				var existing models.NotificationPreference
				if findErr := tx.Where("user_id = ? AND event_pattern = ?", userID, pattern).First(&existing).Error; findErr == nil {
					if updateErr := tx.Model(&existing).Updates(map[string]any{
						"channels":   models.StringList(channels),
						"updated_at": time.Now(),
					}).Error; updateErr != nil {
						return updateErr
					}
				} else {
					pref.CreatedAt = time.Now()
					if createErr := tx.Create(&pref).Error; createErr != nil {
						return createErr
					}
				}
			}
		}
		return nil
	})
}

// ResetPreferences deletes all custom preferences for a user, reverting to system defaults.
func ResetPreferences(ctx context.Context, db *gorm.DB, userID string) error {
	if db == nil {
		return errors.New("notification: nil db")
	}
	if strings.TrimSpace(userID) == "" {
		return errors.New("notification: empty user ID")
	}
	return db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.NotificationPreference{}).Error
}

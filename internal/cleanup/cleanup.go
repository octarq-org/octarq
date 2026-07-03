// Package cleanup runs periodic maintenance: purging expired click events
// based on the data_retention_days setting.
package cleanup

import (
	"context"
	"log"
	"time"

	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm"
)

// Start purges LinkEvents older than the retention window once at startup
// and then every 24 hours. retentionDays is called each cycle so runtime
// changes to the setting take effect without a restart.
// Pass 0 or a negative value to disable purging.
func Start(ctx context.Context, db *gorm.DB, retentionDays func() int) {
	purge := func() {
		days := retentionDays()
		if days <= 0 {
			return
		}
		cutoff := time.Now().AddDate(0, 0, -days)
		
		totalPurged := int64(0)
		for {
			var ids []uint
			if err := db.Model(&models.LinkEvent{}).Where("created_at < ?", cutoff).Limit(2000).Pluck("id", &ids).Error; err != nil {
				log.Printf("cleanup: query link_events: %v", err)
				return
			}
			if len(ids) == 0 {
				break
			}
			
			res := db.Delete(&models.LinkEvent{}, ids)
			if res.Error != nil {
				log.Printf("cleanup: purge link_events batch: %v", res.Error)
				return
			}
			totalPurged += res.RowsAffected
			
			// Yield execution briefly to keep the database responsive
			time.Sleep(50 * time.Millisecond)
		}
		
		if totalPurged > 0 {
			log.Printf("cleanup: purged %d total link_events older than %d days", totalPurged, days)
		}
	}

	purge()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			purge()
		}
	}
}
// StartSessionCleanup deletes expired sessions once at startup and every hour.
// It also removes legacy "Unknown" sessions (empty user_agent) left over from
// old switchOrg calls that used SetSession instead of SetSessionFromRequest.
func StartSessionCleanup(ctx context.Context, db *gorm.DB) {
	purge := func() {
		now := time.Now()
		// Expired sessions
		res := db.Where("expires_at < ?", now).Delete(&models.Session{})
		if res.Error != nil {
			log.Printf("cleanup: purge expired sessions: %v", res.Error)
		} else if res.RowsAffected > 0 {
			log.Printf("cleanup: purged %d expired sessions", res.RowsAffected)
		}
		// Legacy empty-UA sessions (created by old SetSession without IP/UA)
		res2 := db.Where("user_agent = ''").Delete(&models.Session{})
		if res2.Error != nil {
			log.Printf("cleanup: purge empty-UA sessions: %v", res2.Error)
		} else if res2.RowsAffected > 0 {
			log.Printf("cleanup: purged %d legacy empty-UA sessions", res2.RowsAffected)
		}
	}

	purge()
	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			purge()
		}
	}
}

// Package cleanup runs periodic maintenance: purging expired data
// based on the retention window.
package cleanup

import (
	"context"
	"log"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// Start runs provided plugin cleanup functions (e.g. purging LinkEvents)
// once at startup and then every 24 hours. retentionDays is called each cycle
// so runtime changes to the setting take effect without a restart.
// Pass 0 or a negative value to disable purging.
func Start(ctx context.Context, retentionDays func() int, cleanups ...func(ctx context.Context, retentionDays int)) {
	purge := func() {
		days := retentionDays()
		if days <= 0 {
			return
		}
		for _, c := range cleanups {
			c(ctx, days)
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

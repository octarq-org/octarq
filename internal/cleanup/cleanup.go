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

// pruneAuditLogs deletes audit_log rows older than the data_retention_days
// retention window. Rows are deleted in small batches to avoid holding a long
// lock on a potentially large table. A window of 0 or negative disables pruning.
func pruneAuditLogs(db *gorm.DB, days int) {
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	totalPurged := int64(0)
	for {
		var ids []uint
		if err := db.Model(&models.AuditLog{}).Where("created_at < ?", cutoff).Limit(2000).Pluck("id", &ids).Error; err != nil {
			log.Printf("cleanup: prune audit logs: %v", err)
			return
		}
		if len(ids) == 0 {
			break
		}
		res := db.Delete(&models.AuditLog{}, ids)
		if res.Error != nil {
			log.Printf("cleanup: prune audit logs: %v", res.Error)
			return
		}
		totalPurged += res.RowsAffected
		time.Sleep(50 * time.Millisecond)
	}
	if totalPurged > 0 {
		log.Printf("cleanup: purged %d audit log rows older than %d days", totalPurged, days)
	}
}

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
// It additionally prunes audit_log rows older than the data_retention_days
// instance setting (see pruneAuditLogs).
func StartSessionCleanup(ctx context.Context, db *gorm.DB, retentionDays func() int) {
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
		// Audit logs older than the retention window
		pruneAuditLogs(db, retentionDays())
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

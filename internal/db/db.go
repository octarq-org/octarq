// Package db opens the configured database (SQLite or Postgres) through GORM
// and runs migrations. SQLite uses the pure-Go glebarez driver so the final
// binary needs no cgo.
package db

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to the database WITHOUT migrating. Migration is deferred to
// Migrate so that plugin-contributed models (registered after Open) are
// migrated together with the core schema in a single pass — see the plugin
// package for why AutoMigrate must not run at open time.
func Open(cfg *config.Config) (*gorm.DB, error) {
	var dial gorm.Dialector
	switch cfg.DBDriver {
	case "sqlite":
		dsn := cfg.DBDSN
		if !strings.Contains(dsn, "_busy_timeout") && !strings.Contains(dsn, "_pragma") {
			sep := "?"
			if strings.Contains(dsn, "?") {
				sep = "&"
			}
			dsn = fmt.Sprintf("%s%s_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", dsn, sep)
		}
		dial = sqlite.Open(dsn)
	case "postgres":
		dial = postgres.Open(cfg.DBDSN)
	case "mysql":
		dial = mysql.Open(cfg.DBDSN)
	default:
		return nil, fmt.Errorf("unsupported driver %q", cfg.DBDriver)
	}

	dbLogger := logger.New(
		log.New(os.Stderr, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: false,
			Colorful:                  false,
		},
	)
	gdb, err := gorm.Open(dial, &gorm.Config{
		Logger: dbLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if sqlDB, err := gdb.DB(); err == nil {
		if cfg.DBDriver == "sqlite" {
			// For SQLite with WAL mode, pool connections safely with timeout
			sqlDB.SetMaxOpenConns(50)
			sqlDB.SetMaxIdleConns(10)
			sqlDB.SetConnMaxLifetime(time.Hour)
		}
	}

	return gdb, nil
}

// Migrate auto-migrates the core schema plus any extra (plugin) models, then
// runs one-off data migrations. Call this once, after every plugin has been
// registered, before serving traffic.
func Migrate(gdb *gorm.DB, extraModels ...any) error {
	all := append(models.AllModels(), extraModels...)
	if err := gdb.AutoMigrate(all...); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Data migration: move org-level settings from global settings to workspace_settings (for org 1)
	for _, key := range []string{"catch_all", "auto_wrap_links", "reserved_mailboxes"} {
		var s models.Setting
		if err := gdb.Where("key = ?", key).First(&s).Error; err == nil {
			var count int64
			gdb.Model(&models.WorkspaceSetting{}).Where("org_id = ? AND key = ?", 1, key).Count(&count)
			if count == 0 {
				gdb.Create(&models.WorkspaceSetting{OrgID: 1, Key: key, Value: s.Value})
			}
			gdb.Delete(&s)
		}
	}

	// Data migration: backfill User.IsInstanceAdmin for existing installs by
	// seeding the org-1 owner once. The guard on "no user already flagged"
	// keeps it to exactly one run and leaves fresh installs — where the flag is
	// set properly at first login — untouched.
	{
		var flagged int64
		gdb.Model(&models.User{}).Where("is_instance_admin = ?", true).Count(&flagged)
		if flagged == 0 {
			var ownerID uint
			if err := gdb.Model(&models.OrgMember{}).
				Where("org_id = ? AND role = ?", 1, "owner").
				Order("user_id ASC").
				Limit(1).
				Pluck("user_id", &ownerID).Error; err == nil && ownerID != 0 {
				gdb.Model(&models.User{}).Where("id = ?", ownerID).Update("is_instance_admin", true)
			}
		}
	}

	// Data migration: drop sessions that carry no org.
	//
	// Such a session resolves to org 0 and fails closed on every tenant-scoped
	// request — authenticated, but on a screen that looks logged in. Deleting
	// the row sends the user through the normal login flow instead. Nobody
	// loses data, and on any install created after multi-tenancy this deletes
	// nothing.
	gdb.Where("org_id = 0").Delete(&models.Session{})

	// Data migration: make existing tenant subdomains usable link hosts.
	backfillTenantSubdomainLinkHosts(gdb)

	return nil
}

// tenantDomainRow mirrors the dns plugin's Domain for the backfill below. Core
// cannot import the plugin; this is the same bargain internal/tenancy already
// makes for provisioning and origin makes for reads.
type tenantDomainRow struct {
	ID        uint   `gorm:"primaryKey"`
	OrgID     uint   `gorm:"column:owner_id"`
	Name      string `gorm:"size:255"`
	ForLink   bool
	LinkHosts models.HostList `gorm:"type:text"`
}

func (tenantDomainRow) TableName() string { return "domains" }

// backfillTenantSubdomainLinkHosts upgrades existing tenant-subdomain domain
// rows into usable link hosts.
//
// Before Provision wrote ForLink + LinkHosts, every Cloud tenant subdomain row
// was for_link=false with an empty host list — invisible to the links plugin,
// so every tenant's host dropdown was empty and all short links fell into the
// single shared host="" namespace (tenant A's slug blocking tenant B forever,
// plus a 409 existence probe across tenants). This backfills exactly those
// rows: for_link=true plus an enabled LinkHost for the subdomain itself.
//
// Only rows whose name equals <org slug>.<current base domain> are touched —
// a custom domain the user added deliberately, and chose never to serve links
// on, must never be flipped by a migration. When no base domain is configured
// the whole migration is skipped, and the structural guard (for_link=false AND
// no link host) keeps it to a single run: once upgraded a row no longer
// matches the selection.
func backfillTenantSubdomainLinkHosts(gdb *gorm.DB) {
	base := models.BaseDomain(gdb)
	if base == "" || !gdb.Migrator().HasTable("domains") || !gdb.Migrator().HasTable("orgs") {
		return
	}
	var orgs []models.Org
	if err := gdb.Find(&orgs).Error; err != nil {
		return
	}
	slugByOrg := make(map[uint]string, len(orgs))
	for _, o := range orgs {
		slugByOrg[o.ID] = o.Slug
	}
	var doms []tenantDomainRow
	if err := gdb.Where("for_link = ?", false).Find(&doms).Error; err != nil {
		return
	}
	for _, d := range doms {
		if len(d.LinkHosts) > 0 {
			continue
		}
		slug, ok := slugByOrg[d.OrgID]
		if !ok {
			continue
		}
		if d.Name != strings.ToLower(slug)+"."+base {
			continue
		}
		gdb.Model(&tenantDomainRow{}).Where("id = ?", d.ID).Updates(map[string]any{
			"for_link":   true,
			"link_hosts": models.HostList{{Host: d.Name, Enabled: true}},
		})
	}
}

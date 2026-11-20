package db

import (
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"go.opentelemetry.io/otel"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

func dbLoggerConfig(provisioned bool) logger.Config {
	return logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: provisioned,
		Colorful:                  false,
	}
}

var defaultSQLitePragmas = []struct {
	key   string
	param string
}{
	{"busy_timeout", "busy_timeout(5000)"},
	{"journal_mode", "journal_mode(WAL)"},
	{"synchronous", "synchronous(NORMAL)"},
	{"foreign_keys", "foreign_keys(1)"},
}

func EnsureSQLiteWALDSN(dsn string) string {
	base, query, found := strings.Cut(dsn, "?")
	if !found {
		return base + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"
	}

	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return base + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"
	}

	parsed, err := url.ParseQuery(trimmedQuery)
	if err != nil {
		cleanDSN := strings.TrimRight(dsn, "&")
		return cleanDSN + "&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"
	}

	existingPragmas := parsed["_pragma"]
	hasPragma := func(prefix string) bool {
		prefixLower := strings.ToLower(prefix)
		for _, p := range existingPragmas {
			pLower := strings.ToLower(strings.TrimSpace(p))
			if strings.HasPrefix(pLower, prefixLower) {
				return true
			}
		}
		return false
	}

	var missing []string
	for _, item := range defaultSQLitePragmas {
		if !hasPragma(item.key) {
			missing = append(missing, "_pragma="+item.param)
		}
	}

	if len(missing) == 0 {
		return dsn
	}

	cleanDSN := strings.TrimRight(dsn, "&")
	return cleanDSN + "&" + strings.Join(missing, "&")
}

func applySQLitePragmas(sqlDB *sql.DB) error {
	if _, err := sqlDB.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("sqlite busy_timeout: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("sqlite journal_mode: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA synchronous = NORMAL"); err != nil {
		return fmt.Errorf("sqlite synchronous: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("sqlite foreign_keys: %w", err)
	}
	return nil
}

func openSQLiteHandle(cfg *config.Config, meterScope string, maxConns int) (*gorm.DB, error) {
	dsn := EnsureSQLiteWALDSN(cfg.DBDSN)
	dial := sqlite.Open(dsn)

	dbLogger := logger.New(
		log.New(os.Stderr, "\r\n", log.LstdFlags),
		dbLoggerConfig(cfg.Provisioned()),
	)
	gdb, err := gorm.Open(dial, &gorm.Config{
		Logger: dbLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("sql db: %w", err)
	}

	if err := gdb.Use(tracing.NewPlugin(tracing.WithoutQueryVariables())); err != nil {
		slog.Warn("db: failed to install gorm tracing plugin", "err", err)
	}
	if err := telemetry.RegisterDBStatsMetrics(otel.GetMeterProvider().Meter(meterScope), sqlDB); err != nil {
		slog.Warn("db: failed to register db pool metrics", "err", err)
	}
	telemetry.RegisterDB(sqlDB)

	if err := applySQLitePragmas(sqlDB); err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(maxConns)
	sqlDB.SetMaxIdleConns(maxConns)

	return gdb, nil
}

func Open(cfg *config.Config) (*gorm.DB, error) {
	if cfg.DBDriver == "sqlite" {
		return openSQLiteHandle(cfg, "github.com/octarq-org/octarq/server/db", 1)
	}

	var dial gorm.Dialector
	switch cfg.DBDriver {
	case "postgres":
		dial = postgres.Open(cfg.DBDSN)
	case "mysql":
		dial = mysql.Open(cfg.DBDSN)
	default:
		return nil, fmt.Errorf("unsupported driver %q", cfg.DBDriver)
	}

	dbLogger := logger.New(
		log.New(os.Stderr, "\r\n", log.LstdFlags),
		dbLoggerConfig(cfg.Provisioned()),
	)
	gdb, err := gorm.Open(dial, &gorm.Config{
		Logger: dbLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("sql db: %w", err)
	}

	if err := gdb.Use(tracing.NewPlugin(tracing.WithoutQueryVariables())); err != nil {
		slog.Warn("db: failed to install gorm tracing plugin", "err", err)
	}
	if err := telemetry.RegisterDBStatsMetrics(otel.GetMeterProvider().Meter("github.com/octarq-org/octarq/server/db"), sqlDB); err != nil {
		slog.Warn("db: failed to register db pool metrics", "err", err)
	}
	telemetry.RegisterDB(sqlDB)

	return gdb, nil
}

func OpenDualPool(cfg *config.Config) (writeDB *gorm.DB, readDB *gorm.DB, err error) {
	switch cfg.DBDriver {
	case "sqlite":
		writeDB, err = openSQLiteHandle(cfg, "github.com/octarq-org/octarq/server/db", 1)
		if err != nil {
			return nil, nil, fmt.Errorf("open sqlite writer pool: %w", err)
		}

		readerConns := max(4, runtime.NumCPU())
		readDB, err = openSQLiteHandle(cfg, "github.com/octarq-org/octarq/server/db/reader", readerConns)
		if err != nil {
			if wsql, errDB := writeDB.DB(); errDB == nil {
				_ = wsql.Close()
			}
			return nil, nil, fmt.Errorf("open sqlite reader pool: %w", err)
		}

		return writeDB, readDB, nil

	case "postgres", "mysql":
		writeDB, err = Open(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("open writer db: %w", err)
		}

		readDB, err = Open(cfg)
		if err != nil {
			if wsql, errDB := writeDB.DB(); errDB == nil {
				_ = wsql.Close()
			}
			return nil, nil, fmt.Errorf("open reader db: %w", err)
		}

		return writeDB, readDB, nil

	default:
		return nil, nil, fmt.Errorf("unsupported driver %q", cfg.DBDriver)
	}
}

func Migrate(gdb *gorm.DB, extraModels ...any) error {
	all := append(models.AllModels(), extraModels...)
	if err := gdb.AutoMigrate(all...); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	targetKeys := []string{"catch_all", "auto_wrap_links", "reserved_mailboxes"}
	var settings []models.Setting
	if err := gdb.Where("key IN ?", targetKeys).Find(&settings).Error; err == nil && len(settings) > 0 {
		var existingKeys []string
		gdb.Model(&models.WorkspaceSetting{}).Where("org_id = ? AND key IN ?", 1, targetKeys).Pluck("key", &existingKeys)

		existingMap := make(map[string]bool)
		for _, k := range existingKeys {
			existingMap[k] = true
		}

		var newWS []models.WorkspaceSetting

		for _, s := range settings {
			if !existingMap[s.Key] {
				newWS = append(newWS, models.WorkspaceSetting{OrgID: 1, Key: s.Key, Value: s.Value})
			}
		}

		if len(newWS) > 0 {
			gdb.Create(&newWS)
		}
		if len(settings) > 0 {
			gdb.Delete(&settings)
		}
	}

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
				gdb.Model(&models.User{}).Where("id = ?", ownerID).Updates(map[string]any{"is_instance_admin": true, "email_verified": true})
			}
		}
	}

	gdb.Model(&models.User{}).Where("is_instance_admin = ? AND email_verified = ?", true, false).Update("email_verified", true)

	gdb.Where("org_id = 0").Delete(&models.Session{})

	m := gdb.Migrator()
	if tables, err := m.GetTables(); err == nil {
		gdb.Transaction(func(tx *gorm.DB) error {
			for _, table := range []string{"domains", "provider_accounts", "links", "mailboxes", "ddns_tokens", "dns_ddns_tokens", "tokens", "user_tokens", "webhooks", "notification_channels"} {
				if slices.Contains(tables, table) && tx.Migrator().HasColumn(table, "owner_id") {
					tx.Table(table).Where("owner_id = 0 OR owner_id IS NULL").Update("owner_id", 1)
				}
			}
			return nil
		})
	}

	backfillTenantSubdomainLinkHosts(gdb)

	return nil
}

type tenantDomainRow struct {
	ID        uint   `gorm:"primaryKey"`
	OrgID     uint   `gorm:"column:owner_id"`
	Name      string `gorm:"size:255"`
	ForLink   bool
	LinkHosts models.HostList `gorm:"type:text"`
}

func (tenantDomainRow) TableName() string { return "domains" }

func backfillTenantSubdomainLinkHosts(gdb *gorm.DB) {
	gdb.Transaction(func(tx *gorm.DB) error {
		base := models.BaseDomain(tx)
		if base == "" || !tx.Migrator().HasTable("domains") || !tx.Migrator().HasTable("orgs") {
			return nil
		}
		var orgs []models.Org
		if err := tx.Find(&orgs).Error; err != nil {
			return err
		}
		slugByOrg := make(map[uint]string, len(orgs))
		for _, o := range orgs {
			slugByOrg[o.ID] = o.Slug
		}
		var doms []tenantDomainRow
		if err := tx.Where("for_link = ?", false).Find(&doms).Error; err != nil {
			return err
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
			tx.Model(&tenantDomainRow{}).Where("id = ?", d.ID).Updates(map[string]any{
				"for_link":   true,
				"link_hosts": models.HostList{{Host: d.Name, Enabled: true}},
			})
		}
		return nil
	})
}

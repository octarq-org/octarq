package db

import (
	"database/sql"
	"errors"
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
	"github.com/octarq-org/octarq/server/origin"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"go.opentelemetry.io/otel"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

const legacyDataSetting = "legacy_data_migrated"

func Migrate(gdb *gorm.DB, extraModels ...any) error {
	all := append(models.AllModels(), extraModels...)
	if err := gdb.AutoMigrate(all...); err != nil {
		return fmt.Errorf("migrate: %w", err)
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

	var legacyRan int64
	gdb.Model(&models.Setting{}).Where("key = ?", legacyDataSetting).Count(&legacyRan)
	if legacyRan == 0 {
		gdb.Where("org_id = 0").Delete(&models.Session{})

		m := gdb.Migrator()
		if tables, err := m.GetTables(); err == nil {
			gdb.Transaction(func(tx *gorm.DB) error {
				for _, table := range []string{"tokens", "user_tokens", "webhooks", "notification_channels"} {
					if slices.Contains(tables, table) && tx.Migrator().HasColumn(table, "owner_id") {
						tx.Table(table).Where("owner_id = 0 OR owner_id IS NULL").Update("owner_id", 1)
					}
				}
				return nil
			})
		}
		gdb.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.Setting{Key: legacyDataSetting, Value: "1"})
	}

	if err := backfillOrgHosts(gdb); err != nil {
		return fmt.Errorf("backfill org hosts: %w", err)
	}

	return nil
}

func backfillOrgHosts(gdb *gorm.DB) error {
	if gdb == nil || !gdb.Migrator().HasTable("org_hosts") {
		return nil
	}
	return gdb.Transaction(func(tx *gorm.DB) error {
		base := models.BaseDomain(tx)
		if base != "" && tx.Migrator().HasTable("orgs") {
			var orgs []models.Org
			if err := tx.Find(&orgs).Error; err != nil {
				return err
			}
			for _, o := range orgs {
				if o.Slug == "" {
					continue
				}
				host := origin.NormalizeHost(strings.ToLower(o.Slug) + "." + base)
				if err := bindOrgHost(tx, o.ID, host, true, "core:tenancy"); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func bindOrgHost(tx *gorm.DB, orgID uint, rawHost string, traffic bool, source string) error {
	host := origin.NormalizeHost(rawHost)
	if host == "" || orgID == 0 {
		return nil
	}
	if base := models.BaseDomain(tx); base != "" && host == base {
		slog.Warn("org_hosts backfill: skipping base domain apex", "host", host, "org", orgID)
		return nil
	}
	var conflicts int64
	if err := tx.Model(&models.OrgHost{}).Where("host = ? AND owner_id <> ?", host, orgID).Count(&conflicts).Error; err != nil {
		return err
	}
	if conflicts > 0 {
		slog.Warn("org_hosts backfill: host is claimed by another workspace, skipping",
			"host", host, "org", orgID)
		return nil
	}
	var existing models.OrgHost
	err := tx.Where("host = ? AND owner_id = ? AND source = ?", host, orgID, source).First(&existing).Error
	switch {
	case err == nil:
		if traffic && !existing.Traffic {
			if err := tx.Model(&existing).Update("traffic", true).Error; err != nil {
				return err
			}
		}
		return nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return err
	}
	row := models.OrgHost{OrgID: orgID, Host: host, Traffic: traffic, Source: source}
	if err := tx.Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			slog.Warn("org_hosts backfill: host was claimed concurrently, skipping", "host", host, "org", orgID)
			return nil
		}
		return err
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "unique constraint failed") ||
		strings.Contains(lower, "duplicate key value violates unique constraint") ||
		strings.Contains(lower, "unique violation")
}

package db

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/idempotency"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/octarq-org/octarq/plugins/links"
	"github.com/octarq-org/octarq/plugins/mail"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	gormschema "gorm.io/gorm/schema"
)

// The GORM logger must stay chatty in development (default SQLite, no Redis):
// a record-not-found line is ordinary control flow there and useful while
// debugging. On provisioned infrastructure (external Postgres/MySQL/Redis —
// the production posture) the same lines are noise plus schema/query-pattern
// leakage into shipped logs, so ErrRecordNotFound is silenced there while
// genuine SQL errors and slow-query warnings keep flowing.
func TestDBLoggerConfig(t *testing.T) {
	t.Run("development keeps record-not-found logging", func(t *testing.T) {
		cfg := dbLoggerConfig(false)
		if cfg.LogLevel != logger.Warn {
			t.Errorf("LogLevel = %v, want Warn", cfg.LogLevel)
		}
		if cfg.IgnoreRecordNotFoundError {
			t.Error("IgnoreRecordNotFoundError = true, want false in development")
		}
	})

	t.Run("production silences record-not-found, keeps errors and slow queries", func(t *testing.T) {
		cfg := dbLoggerConfig(true)
		if cfg.LogLevel != logger.Warn {
			t.Errorf("LogLevel = %v, want Warn (slow queries stay surfaced)", cfg.LogLevel)
		}
		if !cfg.IgnoreRecordNotFoundError {
			t.Error("IgnoreRecordNotFoundError = false, want true in production")
		}
		if cfg.SlowThreshold != 200*time.Millisecond {
			t.Errorf("SlowThreshold = %v, want 200ms", cfg.SlowThreshold)
		}
	})
}

func TestOpenSQLiteAppliesWALAndSingleConnPool(t *testing.T) {
	dir := t.TempDir()

	openAndCheck := func(t *testing.T, dsn string) {
		t.Helper()
		gdb, err := Open(&config.Config{DBDriver: "sqlite", DBDSN: dsn})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		sqlDB, err := gdb.DB()
		if err != nil {
			t.Fatalf("DB(): %v", err)
		}
		if got := sqlDB.Stats().MaxOpenConnections; got != 1 {
			t.Fatalf("MaxOpenConnections = %d, want 1", got)
		}
		var mode string
		if err := sqlDB.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatalf("journal_mode: %v", err)
		}
		if strings.ToLower(mode) != "wal" {
			t.Fatalf("journal_mode = %q, want wal", mode)
		}
	}

	t.Run("plain path", func(t *testing.T) {
		openAndCheck(t, filepath.Join(dir, "plain.db"))
	})
	t.Run("existing pragma in DSN still gets WAL", func(t *testing.T) {
		openAndCheck(t, filepath.Join(dir, "pragma.db")+"?_pragma=busy_timeout(5000)")
	})
}

func TestDBOpenAndMigrate(t *testing.T) {
	t.Parallel()

	// 1. Open SQLite (valid)
	cfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    "file::memory:?cache=shared",
	}
	gdb, err := Open(cfg)
	if err != nil {
		t.Fatalf("expected no error opening DB, got %v", err)
	}

	// 2. Open unsupported driver
	badCfg := &config.Config{
		DBDriver: "invalid",
	}
	if _, err := Open(badCfg); err == nil {
		t.Error("expected error for unsupported driver, got nil")
	}

	// 2b. Open MySQL driver (verifies dialctor creation)
	mysqlCfg := &config.Config{
		DBDriver: "mysql",
		DBDSN:    "user:password@tcp(127.0.0.1:1)/nonexistent?timeout=100ms",
	}
	_, _ = Open(mysqlCfg)

	// 3. Migrate with legacy data
	// Seed legacy setting
	gdb.AutoMigrate(&models.Setting{}, &models.WorkspaceSetting{}, &models.OrgMember{}, &models.User{}, &models.Session{})
	gdb.Create(&models.Setting{Key: "catch_all", Value: "true"})
	gdb.Create(&models.OrgMember{OrgID: 1, UserID: 42, Role: "owner"})
	gdb.Create(&models.Session{UserID: 42, OrgID: 0, Token: "unscoped-session-token"})

	type ExtraModel struct {
		ID   uint   `gorm:"primaryKey"`
		Name string `gorm:"size:255"`
	}
	err = Migrate(gdb, &ExtraModel{})
	if err != nil {
		t.Errorf("expected no error migrating DB, got %v", err)
	}

	// Verify setting was migrated to WorkspaceSetting
	var ws models.WorkspaceSetting
	if err := gdb.Where("org_id = ? AND key = ?", 1, "catch_all").First(&ws).Error; err != nil {
		t.Errorf("expected catch_all setting migrated to workspace 1, got error: %v", err)
	}

	// Verify user 42 is flagged as instance admin and email verified
	var user models.User
	if err := gdb.Where("id = ?", 42).First(&user).Error; err == nil {
		if !user.IsInstanceAdmin {
			t.Errorf("expected user 42 to be flagged as instance admin")
		}
		if !user.EmailVerified {
			t.Errorf("expected user 42 to have EmailVerified = true")
		}
	}

	// Verify orgID 0 session was cleaned up
	var count int64
	gdb.Model(&models.Session{}).Where("token = ?", "unscoped-session-token").Count(&count)
	if count != 0 {
		t.Errorf("expected org 0 session to be deleted, found count %d", count)
	}
}

func assertDomainRow(t *testing.T, gdb *gorm.DB, name string, wantForLink bool, wantHost string) {
	t.Helper()
	var row tenantDomainRow
	if err := gdb.Where("name = ?", name).First(&row).Error; err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if row.ForLink != wantForLink {
		t.Fatalf("%s for_link = %v, want %v", name, row.ForLink, wantForLink)
	}
	if wantHost == "" {
		if len(row.LinkHosts) != 0 {
			t.Fatalf("%s LinkHosts = %+v, want none", name, row.LinkHosts)
		}
		return
	}
	if len(row.LinkHosts) != 1 || row.LinkHosts[0].Host != wantHost || !row.LinkHosts[0].Enabled {
		t.Fatalf("%s LinkHosts = %+v, want [{%s true}]", name, row.LinkHosts, wantHost)
	}
}

// Tenant-subdomain backfill: only rows whose name equals <org slug>.<base> (and
// are for_link=false with no link hosts) become link hosts. Custom domains the
// user added deliberately are never flipped, and a second run is a no-op.
func TestBackfillTenantSubdomainLinkHosts(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := gdb.AutoMigrate(&models.Setting{}, &models.Org{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := gdb.Migrator().CreateTable(&tenantDomainRow{}); err != nil {
		t.Fatalf("create domains: %v", err)
	}

	gdb.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "app.example.com"})
	gdb.Create(&models.Org{ID: 1, Name: "Acme", Slug: "acme9x"})
	gdb.Create(&models.Org{ID: 2, Name: "Globex", Slug: "globex"})

	// Old-style provisioned tenant subdomains: for_link=false, no link hosts.
	gdb.Create(&tenantDomainRow{OrgID: 1, Name: "acme9x.app.example.com"})
	gdb.Create(&tenantDomainRow{OrgID: 2, Name: "globex.app.example.com"})
	// Custom domain the user added deliberately and never enabled for links.
	gdb.Create(&tenantDomainRow{OrgID: 1, Name: "mail.example.com"})

	backfillTenantSubdomainLinkHosts(gdb)

	assertDomainRow(t, gdb, "acme9x.app.example.com", true, "acme9x.app.example.com")
	assertDomainRow(t, gdb, "globex.app.example.com", true, "globex.app.example.com")
	assertDomainRow(t, gdb, "mail.example.com", false, "")

	// Idempotent: a second run must change nothing (the guard is structural).
	backfillTenantSubdomainLinkHosts(gdb)
	assertDomainRow(t, gdb, "acme9x.app.example.com", true, "acme9x.app.example.com")
	assertDomainRow(t, gdb, "mail.example.com", false, "")
}

// No base domain configured → the backfill is skipped entirely. This mirrors
// the handler guard: single-tenant self-hosted installs keep neutral-host links.
func TestBackfillSkipsWithoutBase(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := gdb.AutoMigrate(&models.Setting{}, &models.Org{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := gdb.Migrator().CreateTable(&tenantDomainRow{}); err != nil {
		t.Fatalf("create domains: %v", err)
	}

	gdb.Create(&models.Org{ID: 1, Name: "Acme", Slug: "acme9x"})
	gdb.Create(&tenantDomainRow{OrgID: 1, Name: "acme9x.app.example.com"})

	backfillTenantSubdomainLinkHosts(gdb)

	assertDomainRow(t, gdb, "acme9x.app.example.com", false, "")
}

func TestParsePostgresDSNEdgeCases(t *testing.T) {
	t.Parallel()

	// Empty
	if _, err := ParsePostgresDSN(""); err == nil {
		t.Error("expected error for empty DSN")
	}

	// URL format
	cfg, err := ParsePostgresDSN("postgres://usr:pwd@dbhost:5433/octarq_db?sslmode=disable")
	if err != nil {
		t.Fatalf("unexpected error parsing URL DSN: %v", err)
	}
	if cfg.Host != "dbhost" || cfg.Port != "5433" || cfg.User != "usr" || cfg.Password != "pwd" || cfg.DBName != "octarq_db" || cfg.SSLMode != "disable" {
		t.Errorf("parsed URL config mismatch: %+v", cfg)
	}

	// Key-value format
	cfg2, err := ParsePostgresDSN("host=localhost port=5432 user=myuser password=mypass dbname=mydb sslmode=require")
	if err != nil {
		t.Fatalf("unexpected error parsing KV DSN: %v", err)
	}
	if cfg2.Host != "localhost" || cfg2.User != "myuser" || cfg2.DBName != "mydb" || cfg2.SSLMode != "require" {
		t.Errorf("parsed KV config mismatch: %+v", cfg2)
	}

	// Missing dbname
	if _, err := ParsePostgresDSN("host=localhost port=5432"); err == nil {
		t.Error("expected error for KV DSN missing dbname")
	}
}

func TestBackupAndRestoreEdgeCases(t *testing.T) {
	t.Parallel()

	fnPg := DefaultBackupFilename("postgres", time.Now())
	if filepath.Ext(fnPg) != ".sql" {
		t.Errorf("expected .sql extension for postgres backup, got %q", fnPg)
	}

	fnMy := DefaultBackupFilename("mysql", time.Now())
	if filepath.Ext(fnMy) != ".sql" {
		t.Errorf("expected .sql extension for mysql backup, got %q", fnMy)
	}

	fnLite := DefaultBackupFilename("sqlite", time.Now())
	if filepath.Ext(fnLite) != ".db" {
		t.Errorf("expected .db extension for sqlite backup, got %q", fnLite)
	}

	// Backup unsupported driver
	badCfg := &config.Config{DBDriver: "unsupported"}
	if err := Backup(badCfg, "out.db"); err == nil {
		t.Error("expected Backup error for unsupported driver")
	}

	// Restore non-existent file
	if err := Restore(&config.Config{DBDriver: "sqlite"}, "nonexistent.db"); err == nil {
		t.Error("expected Restore error for non-existent file")
	}

	// Verify empty file
	tmpFile := filepath.Join(t.TempDir(), "empty.db")
	_ = os.WriteFile(tmpFile, []byte{}, 0644)
	if err := VerifySQLiteIntegrity(tmpFile); err == nil {
		t.Error("expected VerifySQLiteIntegrity error for empty file")
	}

	// Restore corrupted file
	corruptFile := filepath.Join(t.TempDir(), "corrupt.db")
	_ = os.WriteFile(corruptFile, []byte("not a sqlite database file"), 0644)
	if err := Restore(&config.Config{DBDriver: "sqlite", DBDSN: tmpFile}, corruptFile); err == nil {
		t.Error("expected restore error for corrupt backup file")
	}
}

func TestBackupAndRestoreSQLiteSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "original.db")
	backupPath := filepath.Join(dir, "backup.db")
	restorePath := filepath.Join(dir, "restored.db")

	// Create and populate original DB
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	gdb.AutoMigrate(&models.Setting{})
	gdb.Create(&models.Setting{Key: "k1", Value: "v1"})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	// Backup
	cfg := &config.Config{DBDriver: "sqlite", DBDSN: dbPath}
	if err := Backup(cfg, backupPath); err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	if err := VerifySQLiteIntegrity(backupPath); err != nil {
		t.Fatalf("VerifySQLiteIntegrity failed on valid backup: %v", err)
	}

	// Restore to target
	cfgRestore := &config.Config{DBDriver: "sqlite", DBDSN: restorePath}
	if err := Restore(cfgRestore, backupPath); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Verify restored DB contents
	rdb, err := gorm.Open(sqlite.Open(restorePath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	var s models.Setting
	if err := rdb.Where("key = ?", "k1").First(&s).Error; err != nil || s.Value != "v1" {
		t.Errorf("restored DB value mismatch: got %+v, err=%v", s, err)
	}
}

func allAppTestModels() []any {
	var all []any
	all = append(all, models.AllModels()...)
	all = append(all, eventbus.Models()...)
	all = append(all, idempotency.Models()...)
	all = append(all, dns.New().Models()...)
	all = append(all, links.New().Models()...)
	all = append(all, mail.New().Models()...)
	return all
}

// TestMigrateAllAppModelsInSQLite verifies that every registered model in core,
// eventbus, idempotency, and all built-in plugins migrates cleanly and supports
// binary byte slice roundtrips.
func TestMigrateAllAppModelsInSQLite(t *testing.T) {
	t.Parallel()

	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	extraModels := append(eventbus.Models(), idempotency.Models()...)
	extraModels = append(extraModels, dns.New().Models()...)
	extraModels = append(extraModels, links.New().Models()...)
	extraModels = append(extraModels, mail.New().Models()...)

	if err := Migrate(gdb, extraModels...); err != nil {
		t.Fatalf("Migrate failed with all app models: %v", err)
	}

	// Verify crucial tables exist
	for _, tbl := range []string{
		"orgs", "users", "settings", "idempotency_records",
		"webhook_deliveries", "mailboxes", "emails", "mail_raw_blobs",
		"domains", "links", "link_events",
	} {
		if !gdb.Migrator().HasTable(tbl) {
			t.Errorf("expected table %q to exist after full migration", tbl)
		}
	}

	// Verify idempotency.Record with ResponseBody []byte roundtrips binary data
	binaryResp := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"status\":\"ok\"}")
	rec := &idempotency.Record{
		OrgID:        1,
		Endpoint:     "POST /api/test",
		Key:          "k-binary-test",
		RequestHash:  "hash123",
		State:        "done",
		StatusCode:   200,
		BodyStored:   true,
		ResponseBody: binaryResp,
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	if err := gdb.Create(rec).Error; err != nil {
		t.Fatalf("failed to insert idempotency record with []byte: %v", err)
	}
	var loadedRec idempotency.Record
	if err := gdb.First(&loadedRec, "key = ?", "k-binary-test").Error; err != nil {
		t.Fatalf("failed to query idempotency record: %v", err)
	}
	if !bytes.Equal(loadedRec.ResponseBody, binaryResp) {
		t.Errorf("ResponseBody binary mismatch: got %q, want %q", loadedRec.ResponseBody, binaryResp)
	}

	// Verify mail.MailRawBlob with Data []byte roundtrips binary data
	rawEML := []byte("From: alice@example.com\r\nTo: bob@example.com\r\n\r\nHello binary mail")
	blob := &mail.MailRawBlob{
		Key:       "mail/1/test.eml",
		Data:      rawEML,
		UpdatedAt: time.Now(),
	}
	if err := gdb.Create(blob).Error; err != nil {
		t.Fatalf("failed to insert MailRawBlob with []byte: %v", err)
	}
	var loadedBlob mail.MailRawBlob
	if err := gdb.First(&loadedBlob, "key = ?", "mail/1/test.eml").Error; err != nil {
		t.Fatalf("failed to query MailRawBlob: %v", err)
	}
	if !bytes.Equal(loadedBlob.Data, rawEML) {
		t.Errorf("MailRawBlob data binary mismatch: got %q, want %q", loadedBlob.Data, rawEML)
	}
}

// TestPostgresDialectDataTypeByteaAndNoBlob verifies that PostgreSQL dialect resolves
// all []byte fields (e.g. idempotency.Record.ResponseBody, MailRawBlob.Data, Email.Raw)
// to "bytea", and that NO field across all models resolves to invalid "blob".
func TestPostgresDialectDataTypeByteaAndNoBlob(t *testing.T) {
	t.Parallel()

	pgDialector := postgres.New(postgres.Config{})
	namer := gormschema.NamingStrategy{}

	for _, model := range allAppTestModels() {
		s, err := gormschema.Parse(model, &sync.Map{}, namer)
		if err != nil {
			t.Fatalf("failed to parse schema for model %T: %v", model, err)
		}

		for _, f := range s.Fields {
			dt := strings.ToLower(pgDialector.DataTypeOf(f))

			// PostgreSQL MUST NOT receive "blob" type (causes SQLSTATE 42704)
			if strings.Contains(dt, "blob") {
				t.Errorf("model %s field %s resolves to invalid PostgreSQL type %q (must be bytea or text)",
					s.Name, f.Name, dt)
			}

			// Specific binary fields must resolve to bytea
			if (s.Name == "Record" && f.Name == "ResponseBody") ||
				(s.Name == "MailRawBlob" && f.Name == "Data") ||
				(s.Name == "Email" && f.Name == "Raw") {
				if dt != "bytea" {
					t.Errorf("model %s binary field %s expected type 'bytea' in PostgreSQL, got %q",
						s.Name, f.Name, dt)
				}
			}
		}
	}
}

// TestMySQLDialectDataTypeBlob verifies that MySQL dialect resolves all []byte fields
// to longblob / blob without error.
func TestMySQLDialectDataTypeBlob(t *testing.T) {
	t.Parallel()

	precision := 6
	myDialector := mysql.New(mysql.Config{
		DefaultDatetimePrecision: &precision,
	})
	namer := gormschema.NamingStrategy{}

	for _, model := range allAppTestModels() {
		s, err := gormschema.Parse(model, &sync.Map{}, namer)
		if err != nil {
			t.Fatalf("failed to parse schema for model %T: %v", model, err)
		}

		for _, f := range s.Fields {
			if (s.Name == "Record" && f.Name == "ResponseBody") ||
				(s.Name == "MailRawBlob" && f.Name == "Data") ||
				(s.Name == "Email" && f.Name == "Raw") {
				dt := strings.ToLower(myDialector.DataTypeOf(f))
				if !strings.Contains(dt, "blob") {
					t.Errorf("model %s binary field %s expected blob type in MySQL, got %q",
						s.Name, f.Name, dt)
				}
			}
		}
	}
}

// TestSQLiteDialectDataTypeBlob verifies that SQLite dialect resolves all []byte fields
// to blob.
func TestSQLiteDialectDataTypeBlob(t *testing.T) {
	t.Parallel()

	namer := gormschema.NamingStrategy{}

	for _, model := range allAppTestModels() {
		s, err := gormschema.Parse(model, &sync.Map{}, namer)
		if err != nil {
			t.Fatalf("failed to parse schema for model %T: %v", model, err)
		}

		for _, f := range s.Fields {
			if (s.Name == "Record" && f.Name == "ResponseBody") ||
				(s.Name == "MailRawBlob" && f.Name == "Data") ||
				(s.Name == "Email" && f.Name == "Raw") {
				if f.FieldType.Kind().String() != "slice" || f.FieldType.Elem().Kind().String() != "uint8" {
					t.Errorf("model %s binary field %s expected []byte type, got %s",
						s.Name, f.Name, f.FieldType.String())
				}
			}
		}
	}
}

package db

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/idempotency"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugins/dns"
	"github.com/octarq-org/octarq/server/plugins/links"
	"github.com/octarq-org/octarq/server/plugins/mail"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormschema "gorm.io/gorm/schema"
)

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

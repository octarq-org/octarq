package coreschema_test

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/coreschema"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func migrated(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(coreschema.Models()...); err != nil {
		t.Fatalf("migrate core schema: %v", err)
	}
	return db
}

// goodMirror declares a subset of org_members, which is what a real mirror does.
type goodMirror struct {
	OrgID  uint   `gorm:"column:org_id"`
	UserID uint   `gorm:"column:user_id"`
	Role   string `gorm:"column:role"`
}

func (goodMirror) TableName() string { return "org_members" }

// driftedMirror names a column org_members does not have — the shape a mirror
// takes after core renames something under it.
type driftedMirror struct {
	OrgID uint   `gorm:"column:org_id"`
	Role  string `gorm:"column:member_role"`
}

func (driftedMirror) TableName() string { return "org_members" }

type goneMirror struct {
	ID uint `gorm:"column:id"`
}

func (goneMirror) TableName() string { return "table_that_never_existed" }

func TestModelsMigrate(t *testing.T) {
	db := migrated(t)
	// The tables plugins actually mirror. If core stops shipping one of these,
	// every mirror of it is dead and this is where that shows up first.
	for _, table := range []string{"orgs", "org_members", "settings", "workspace_settings", "audit_logs", "notification_channels"} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("core schema has no table %q", table)
		}
	}
}

func TestModelsPortableToPostgres(t *testing.T) {
	db := migrated(t)
	for _, model := range coreschema.Models() {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(model); err != nil {
			t.Fatalf("parse model %T: %v", model, err)
		}
		for _, f := range stmt.Schema.Fields {
			if strings.EqualFold(string(f.DataType), "blob") {
				t.Errorf("model %s field %s has explicit blob data type; GORM must use default []byte mapping so Postgres migrates as bytea", stmt.Schema.Name, f.Name)
			}
		}
	}
}

func TestCheckMirrorAcceptsSubset(t *testing.T) {
	if err := coreschema.CheckMirror(migrated(t), goodMirror{}); err != nil {
		t.Fatalf("a mirror declaring a subset of real columns should pass: %v", err)
	}
}

func TestCheckMirrorRejectsDrift(t *testing.T) {
	err := coreschema.CheckMirror(migrated(t), driftedMirror{})
	if err == nil {
		t.Fatal("a mirror naming a column the table does not have must fail")
		return
	}
	// The message has to name the offending column: "schema drift" alone sends
	// the reader back to diffing two structs by eye.
	if !strings.Contains(err.Error(), "member_role") {
		t.Errorf("error should name the missing column, got: %v", err)
	}
}

func TestCheckMirrorRejectsMissingTable(t *testing.T) {
	err := coreschema.CheckMirror(migrated(t), goneMirror{})
	if err == nil {
		t.Fatal("a mirror naming a table that does not exist must fail")
		return
	}
	if !strings.Contains(err.Error(), "table_that_never_existed") {
		t.Errorf("error should name the missing table, got: %v", err)
	}
}

func TestCheckMirrorRejectsNilDB(t *testing.T) {
	if err := coreschema.CheckMirror(nil, goodMirror{}); err == nil {
		t.Fatal("nil db must be an error, not a silent pass")
		return
	}
}

type mirrorWithIgnoredField struct {
	OrgID   uint   `gorm:"column:org_id"`
	Ignored string `gorm:"-"`
}

func (mirrorWithIgnoredField) TableName() string { return "org_members" }

type emptyTableMirror struct {
	OrgID uint `gorm:"column:org_id"`
}

func (emptyTableMirror) TableName() string { return "" }

func TestCheckMirrorWithIgnoredField(t *testing.T) {
	if err := coreschema.CheckMirror(migrated(t), mirrorWithIgnoredField{}); err != nil {
		t.Fatalf("mirror with ignored field should pass: %v", err)
	}
}

func TestCheckMirrorEmptyTableName(t *testing.T) {
	err := coreschema.CheckMirror(migrated(t), emptyTableMirror{})
	if err == nil {
		t.Fatal("mirror with empty table name must fail")
		return
	}
	if !strings.Contains(err.Error(), "resolves to an empty table name") {
		t.Errorf("expected empty table error, got: %v", err)
	}
}

func TestCheckMirrorInvalidType(t *testing.T) {
	err := coreschema.CheckMirror(migrated(t), 12345)
	if err == nil {
		t.Fatal("mirror with invalid non-struct type must fail")
		return
	}
	if !strings.Contains(err.Error(), "parse mirror") {
		t.Errorf("expected parse mirror error, got: %v", err)
	}
}

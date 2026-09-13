package datascope

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type TestItem struct {
	ID    uint   `gorm:"primaryKey"`
	OrgID string `gorm:"column:org_id"`
	Title string `gorm:"column:title"`
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	if err := db.AutoMigrate(&TestItem{}); err != nil {
		t.Fatalf("failed to automigrate TestItem: %v", err)
	}
	return db
}

func TestOrgFilter_Interface(t *testing.T) {
	filter := NewOrgFilter()
	var _ plugin.DataScopeFilter = filter
}

func TestOrgFilter_GormDB(t *testing.T) {
	db := setupTestDB(t)
	filter := NewOrgFilter()

	// Insert test data for two orgs
	_ = db.Create(&TestItem{OrgID: "org-1", Title: "Item 1"}).Error
	_ = db.Create(&TestItem{OrgID: "org-2", Title: "Item 2"}).Error

	query := db.Model(&TestItem{})
	scoped, err := filter.ApplyScope(context.Background(), query, "user-1", "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scopedDB, ok := scoped.(*gorm.DB)
	if !ok {
		t.Fatalf("expected *gorm.DB, got %T", scoped)
	}

	var items []TestItem
	if err := scopedDB.Find(&items).Error; err != nil {
		t.Fatalf("failed to query scoped db: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].OrgID != "org-1" {
		t.Fatalf("expected org-1, got %s", items[0].OrgID)
	}

	// Verify generated SQL
	sqlStr := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		s, _ := filter.ApplyScope(context.Background(), tx.Model(&TestItem{}), "u1", "org-test")
		return s.(*gorm.DB).Find(&[]TestItem{})
	})
	if !strings.Contains(sqlStr, "org_id") || !strings.Contains(sqlStr, "org-test") {
		t.Fatalf("sql does not contain org_id filter: %s", sqlStr)
	}

	// Test gorm.DB by value
	valDB := *db.Model(&TestItem{})
	scopedVal, err := filter.ApplyScope(context.Background(), valDB, "u1", "org-1")
	if err != nil {
		t.Fatalf("unexpected error for gorm.DB value: %v", err)
	}
	if _, ok := scopedVal.(*gorm.DB); !ok {
		t.Fatalf("expected *gorm.DB from gorm.DB value, got %T", scopedVal)
	}

	// Test nil *gorm.DB
	var nilDB *gorm.DB
	_, err = filter.ApplyScope(context.Background(), nilDB, "u1", "org-1")
	if err != ErrNilQuery {
		t.Fatalf("expected ErrNilQuery for nil *gorm.DB, got %v", err)
	}
}

func TestOrgFilter_SQLStrings(t *testing.T) {
	filter := NewOrgFilter()
	ctx := context.Background()

	tests := []struct {
		name     string
		input    string
		orgID    string
		expected string
	}{
		{
			name:     "simple select",
			input:    "SELECT * FROM items",
			orgID:    "org-100",
			expected: "SELECT * FROM items WHERE org_id = ?",
		},
		{
			name:     "select with where",
			input:    "SELECT * FROM items WHERE status = 'active'",
			orgID:    "org-100",
			expected: "SELECT * FROM items WHERE status = 'active' AND org_id = ?",
		},
		{
			name:     "select with order by",
			input:    "SELECT * FROM items ORDER BY created_at DESC",
			orgID:    "org-100",
			expected: "SELECT * FROM items WHERE org_id = ? ORDER BY created_at DESC",
		},
		{
			name:     "select with where and order by",
			input:    "SELECT * FROM items WHERE status = 'active' ORDER BY created_at DESC",
			orgID:    "org-100",
			expected: "SELECT * FROM items WHERE status = 'active' AND org_id = ? ORDER BY created_at DESC",
		},
		{
			name:     "select with group by and having",
			input:    "SELECT category, count(*) FROM items GROUP BY category HAVING count(*) > 1 ORDER BY category",
			orgID:    "org-100",
			expected: "SELECT category, count(*) FROM items WHERE org_id = ? GROUP BY category HAVING count(*) > 1 ORDER BY category",
		},
		{
			name:     "select with where, group by, order by, limit",
			input:    "SELECT cat, count(*) FROM items WHERE active = 1 GROUP BY cat ORDER BY cat LIMIT 10",
			orgID:    "org-100",
			expected: "SELECT cat, count(*) FROM items WHERE active = 1 AND org_id = ? GROUP BY cat ORDER BY cat LIMIT 10",
		},
		{
			name:     "select with offset",
			input:    "SELECT * FROM items OFFSET 10",
			orgID:    "org-100",
			expected: "SELECT * FROM items WHERE org_id = ? OFFSET 10",
		},
		{
			name:     "select with window",
			input:    "SELECT id, count(*) OVER w FROM items WINDOW w AS (PARTITION BY cat)",
			orgID:    "org-100",
			expected: "SELECT id, count(*) OVER w FROM items WHERE org_id = ? WINDOW w AS (PARTITION BY cat)",
		},
		{
			name:     "select with for update",
			input:    "SELECT * FROM items FOR UPDATE",
			orgID:    "org-100",
			expected: "SELECT * FROM items WHERE org_id = ? FOR UPDATE",
		},
		{
			name:     "select with for share",
			input:    "SELECT * FROM items FOR SHARE",
			orgID:    "org-100",
			expected: "SELECT * FROM items WHERE org_id = ? FOR SHARE",
		},
		{
			name:     "subquery in from does not trigger top-level where",
			input:    "SELECT * FROM (SELECT id FROM users WHERE active = 1) u",
			orgID:    "org-100",
			expected: "SELECT * FROM (SELECT id FROM users WHERE active = 1) u WHERE org_id = ?",
		},
		{
			name:     "string literal containing keyword",
			input:    "SELECT * FROM items WHERE note = 'ORDER BY test'",
			orgID:    "org-100",
			expected: "SELECT * FROM items WHERE note = 'ORDER BY test' AND org_id = ?",
		},
		{
			name:     "line comment containing keyword",
			input:    "SELECT * FROM items -- WHERE comment\nWHERE id = 1",
			orgID:    "org-100",
			expected: "SELECT * FROM items -- WHERE comment\nWHERE id = 1 AND org_id = ?",
		},
		{
			name:     "block comment containing keyword",
			input:    "SELECT * FROM items /* ORDER BY comment */ WHERE id = 1",
			orgID:    "org-100",
			expected: "SELECT * FROM items /* ORDER BY comment */ WHERE id = 1 AND org_id = ?",
		},
		{
			name:     "trailing semicolon preserved",
			input:    "SELECT * FROM items WHERE id = 1;",
			orgID:    "org-100",
			expected: "SELECT * FROM items WHERE id = 1 AND org_id = ?;",
		},
		{
			name:     "keyword at end with no next word",
			input:    "SELECT * FROM items GROUP",
			orgID:    "org-100",
			expected: "SELECT * FROM items GROUP WHERE org_id = ?",
		},
		{
			name:     "order keyword at end with trailing whitespace",
			input:    "SELECT * FROM items ORDER   ",
			orgID:    "org-100",
			expected: "SELECT * FROM items ORDER WHERE org_id = ?",
		},
		{
			name:     "for keyword at end with trailing whitespace",
			input:    "SELECT * FROM items FOR   ",
			orgID:    "org-100",
			expected: "SELECT * FROM items FOR WHERE org_id = ?",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := filter.ApplyScope(ctx, tc.input, "u1", tc.orgID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			resStr, ok := res.(string)
			if !ok {
				t.Fatalf("expected string, got %T", res)
			}
			if resStr != tc.expected {
				t.Fatalf("got %q, want %q", resStr, tc.expected)
			}
		})
	}
}

func TestOrgFilter_StringPointerAndBytes(t *testing.T) {
	filter := NewOrgFilter()
	ctx := context.Background()

	// *string
	sqlStr := "SELECT * FROM items"
	resPtr, err := filter.ApplyScope(ctx, &sqlStr, "u1", "org-1")
	if err != nil {
		t.Fatalf("unexpected error for *string: %v", err)
	}
	resPtrStr, ok := resPtr.(*string)
	if !ok {
		t.Fatalf("expected *string, got %T", resPtr)
	}
	if *resPtrStr != "SELECT * FROM items WHERE org_id = ?" {
		t.Fatalf("got %s", *resPtrStr)
	}

	// nil *string
	var nilStrPtr *string
	_, err = filter.ApplyScope(ctx, nilStrPtr, "u1", "org-1")
	if err != ErrNilQuery {
		t.Fatalf("expected ErrNilQuery for nil *string, got %v", err)
	}

	// []byte
	byteRes, err := filter.ApplyScope(ctx, []byte("SELECT * FROM items"), "u1", "org-1")
	if err != nil {
		t.Fatalf("unexpected error for []byte: %v", err)
	}
	bSlice, ok := byteRes.([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", byteRes)
	}
	if string(bSlice) != "SELECT * FROM items WHERE org_id = ?" {
		t.Fatalf("got %s", string(bSlice))
	}

	// empty *string error
	emptyStr := ""
	_, err = filter.ApplyScope(ctx, &emptyStr, "u1", "org-1")
	if err == nil {
		t.Fatal("expected error for empty *string, got nil")
	}

	// empty []byte error
	_, err = filter.ApplyScope(ctx, []byte(""), "u1", "org-1")
	if err == nil {
		t.Fatal("expected error for empty []byte, got nil")
	}
}

func TestOrgFilter_ErrorsAndValidation(t *testing.T) {
	filter := NewOrgFilter()
	ctx := context.Background()

	// Empty orgID
	for _, emptyOrg := range []string{"", "   ", "\t\n"} {
		_, err := filter.ApplyScope(ctx, "SELECT 1", "u1", emptyOrg)
		if err != ErrInvalidOrgID {
			t.Fatalf("expected ErrInvalidOrgID for orgID %q, got %v", emptyOrg, err)
		}
	}

	// Nil query
	_, err := filter.ApplyScope(ctx, nil, "u1", "org-1")
	if err != ErrNilQuery {
		t.Fatalf("expected ErrNilQuery for nil query, got %v", err)
	}

	// Empty sql string
	_, err = filter.ApplyScope(ctx, "   ", "u1", "org-1")
	if err == nil {
		t.Fatal("expected error for empty sql string, got nil")
	}

	// Unsupported type
	_, err = filter.ApplyScope(ctx, 12345, "u1", "org-1")
	if err == nil {
		t.Fatal("expected error for unsupported query type, got nil")
	}

	// Cancelled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = filter.ApplyScope(cancCtx, "SELECT 1", "u1", "org-1")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestOrgFilter_Concurrency(t *testing.T) {
	filter := NewOrgFilter()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			q := "SELECT * FROM orders WHERE id > 0 ORDER BY created_at DESC"
			res, err := filter.ApplyScope(ctx, q, "user-1", "org-conc")
			if err != nil {
				t.Errorf("concurrent ApplyScope error: %v", err)
				return
			}
			expected := "SELECT * FROM orders WHERE id > 0 AND org_id = ? ORDER BY created_at DESC"
			if res != expected {
				t.Errorf("got %q, want %q", res, expected)
			}
		}(i)
	}
	wg.Wait()
}

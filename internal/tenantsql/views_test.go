package tenantsql_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/octarq-org/octarq/internal/tenantsql"
	"github.com/octarq-org/octarq/plugin"
)

func TestRegistry_RegisterAndLookup(t *testing.T) {
	reg := tenantsql.NewRegistry()

	view := plugin.TenantView{
		Name: "tenant_sample",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer", Description: "ID"},
			{Name: "secret", Type: "text", Description: "Secret"},
		},
		Sensitive: []string{"secret"},
		Definition: func(orgID uint) string {
			return fmt.Sprintf("SELECT id, secret FROM sample WHERE org_id = %d", orgID)
		},
	}

	if err := reg.Register(view); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Lookup exact match
	got, ok := reg.Lookup("tenant_sample")
	if !ok {
		t.Fatal("expected view to be found")
	}
	if got.Name != "tenant_sample" {
		t.Errorf("got name %q, want tenant_sample", got.Name)
	}

	// Lookup case-insensitive
	gotCI, ok := reg.Lookup("TENANT_SAMPLE")
	if !ok {
		t.Fatal("expected case-insensitive lookup to succeed")
	}
	if gotCI.Name != "tenant_sample" {
		t.Errorf("got name %q, want tenant_sample", gotCI.Name)
	}

	// Lookup non-existent
	if _, ok := reg.Lookup("tenant_missing"); ok {
		t.Error("expected non-existent view to return false")
	}
}

func TestRegistry_Validation(t *testing.T) {
	reg := tenantsql.NewRegistry()

	// Empty name
	err := reg.Register(plugin.TenantView{
		Name:       "",
		Definition: func(orgID uint) string { return "SELECT 1" },
	})
	if err == nil {
		t.Error("expected error on empty view name")
	}

	// Invalid prefix (missing tenant_)
	err = reg.Register(plugin.TenantView{
		Name:       "links",
		Definition: func(orgID uint) string { return "SELECT 1" },
	})
	if err == nil {
		t.Error("expected error on missing tenant_ prefix")
	}

	// Nil definition
	err = reg.Register(plugin.TenantView{
		Name:       "tenant_nil_def",
		Definition: nil,
	})
	if err == nil {
		t.Error("expected error on nil definition")
	}

	// Invalid identifier name (SQL injection attempt / illegal characters)
	err = reg.Register(plugin.TenantView{
		Name:       "tenant_foo;drop",
		Definition: func(orgID uint) string { return "SELECT 1" },
	})
	if err == nil {
		t.Error("expected error on invalid view name 'tenant_foo;drop'")
	}

	err = reg.Register(plugin.TenantView{
		Name:       "tenant_foo drop",
		Definition: func(orgID uint) string { return "SELECT 1" },
	})
	if err == nil {
		t.Error("expected error on invalid view name 'tenant_foo drop'")
	}

	// Duplicate registration
	valid := plugin.TenantView{
		Name:       "tenant_dup",
		Definition: func(orgID uint) string { return "SELECT 1" },
	}
	if err := reg.Register(valid); err != nil {
		t.Fatalf("initial register failed: %v", err)
	}
	err = reg.Register(valid)
	if err == nil {
		t.Error("expected error on duplicate view registration")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := tenantsql.NewRegistry()

	views := []plugin.TenantView{
		{Name: "tenant_c", Definition: func(orgID uint) string { return "SELECT 3" }},
		{Name: "tenant_a", Definition: func(orgID uint) string { return "SELECT 1" }},
		{Name: "tenant_b", Definition: func(orgID uint) string { return "SELECT 2" }},
	}

	for _, v := range views {
		if err := reg.Register(v); err != nil {
			t.Fatalf("Register(%s) failed: %v", v.Name, err)
		}
	}

	list := reg.List()
	if len(list) != 3 {
		t.Fatalf("got %d views, want 3", len(list))
	}

	// Should be sorted alphabetically
	expected := []string{"tenant_a", "tenant_b", "tenant_c"}
	for i, exp := range expected {
		if list[i].Name != exp {
			t.Errorf("list[%d].Name = %q, want %q", i, list[i].Name, exp)
		}
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := tenantsql.NewRegistry()

	var wg sync.WaitGroup
	workers := 16
	iterations := 100

	// Concurrent registers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				name := fmt.Sprintf("tenant_w%d_item%d", workerID, j)
				_ = reg.Register(plugin.TenantView{
					Name:       name,
					Definition: func(orgID uint) string { return "SELECT 1" },
				})
			}
		}(i)
	}

	// Concurrent lookups and lists
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				name := fmt.Sprintf("tenant_w%d_item%d", workerID, j)
				_, _ = reg.Lookup(name)
				_ = reg.List()
			}
		}(i)
	}

	wg.Wait()
}

func TestDefaultRegistry(t *testing.T) {
	def := tenantsql.DefaultRegistry()
	if def == nil {
		t.Fatal("DefaultRegistry returned nil")
	}
}

func TestRegistry_Reset(t *testing.T) {
	reg := tenantsql.NewRegistry()
	_ = reg.Register(plugin.TenantView{
		Name:       "tenant_x",
		Definition: func(orgID uint) string { return "SELECT 1" },
	})
	if len(reg.List()) != 1 {
		t.Fatalf("expected 1 view, got %d", len(reg.List()))
	}
	reg.Reset()
	if len(reg.List()) != 0 {
		t.Fatalf("expected 0 views after reset, got %d", len(reg.List()))
	}
}

func TestExtractReferencedViews_EdgeCases(t *testing.T) {
	// Empty SQL
	_, err := tenantsql.ExtractReferencedViews("")
	if err == nil {
		t.Error("expected error for empty SQL")
	}

	// Exceeds max length
	longSQL := strings.Repeat("SELECT 1 FROM tenant_links; ", 500)
	_, err = tenantsql.ExtractReferencedViews(longSQL)
	if err == nil {
		t.Error("expected error for overlong SQL")
	}

	// Parse error
	_, err = tenantsql.ExtractReferencedViews("SELECT FROM WHERE")
	if err == nil {
		t.Error("expected error for parse failure")
	}

	// Non-select statement
	_, err = tenantsql.ExtractReferencedViews("INSERT INTO tenant_links (slug) VALUES ('x')")
	if err == nil {
		t.Error("expected error for INSERT statement")
	}

	// Valid with multiple and duplicate views
	views, err := tenantsql.ExtractReferencedViews("SELECT * FROM tenant_links l JOIN tenant_link_events e ON l.id = e.link_id WHERE l.id IN (SELECT link_id FROM tenant_link_events)")
	if err != nil {
		t.Fatalf("valid extraction failed: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 unique views, got %d: %v", len(views), views)
	}
	if views[0] != "tenant_links" || views[1] != "tenant_link_events" {
		t.Errorf("unexpected extracted views order: %v", views)
	}
}

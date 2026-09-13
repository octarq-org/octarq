package plugin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// TestDeclarePermIsIdempotent verifies that calling DeclarePerm multiple times
// with the same Key replaces the definition rather than appending it.
func TestDeclarePermIsIdempotent(t *testing.T) {
	t.Cleanup(plugin.ResetPermRegistry)
	plugin.ResetPermRegistry()

	p1 := plugin.Perm{
		Key:      "dns.records.delete",
		Module:   "dns",
		Resource: "DNS Record",
		Action:   "delete",
		Label:    "Delete a DNS Record (v1)",
		Default:  "admin",
	}
	p2 := plugin.Perm{
		Key:      "dns.records.delete",
		Module:   "dns",
		Resource: "DNS Record",
		Action:   "delete",
		Label:    "Delete a DNS Record (v2)",
		Default:  "admin",
	}

	plugin.DeclarePerm(p1)
	plugin.DeclarePerm(p2)

	declared := plugin.DeclaredPerms()
	if n := len(declared); n != 1 {
		t.Fatalf("declared %d entries for same key, expected 1", n)
	}
	if declared[0].Label != "Delete a DNS Record (v2)" {
		t.Fatalf("expected updated label %q, got %q", "Delete a DNS Record (v2)", declared[0].Label)
	}
}

// TestHasPermFailClosedWhenNil verifies that HasPerm returns false (fail-closed)
// when Context or Context.RequirePerm is nil.
func TestHasPermFailClosedWhenNil(t *testing.T) {
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}

	var nilCtx *plugin.Context
	if nilCtx.HasPerm(req, "dns.records.delete", "admin") {
		t.Fatal("expected nil Context.HasPerm to return false, got true")
	}

	unwiredCtx := &plugin.Context{RequirePerm: nil}
	if unwiredCtx.HasPerm(req, "dns.records.delete", "admin") {
		t.Fatal("expected unwired Context.HasPerm to return false, got true")
	}
}

// TestPermSerializesTheNamesTheMatrixReads pins the exact JSON tag names expected
// by downstream roles matrix UI.
func TestPermSerializesTheNamesTheMatrixReads(t *testing.T) {
	p := plugin.Perm{
		Key:      "dns.records.delete",
		Module:   "dns",
		Resource: "DNS Record",
		Action:   "delete",
		Label:    "Delete a DNS Record",
		Default:  "admin",
	}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	want := map[string]string{
		"key":      "dns.records.delete",
		"module":   "dns",
		"resource": "DNS Record",
		"action":   "delete",
		"label":    "Delete a DNS Record",
		"default":  "admin",
	}

	for name, wantVal := range want {
		gotVal, ok := got[name]
		if !ok {
			t.Errorf("permissions API does not send %q — the matrix reads perm.%s and gets undefined", name, name)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("field %q = %v, want %v", name, gotVal, wantVal)
		}
	}

	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("unexpected wire field %q — the matrix will not read it", name)
		}
	}
}

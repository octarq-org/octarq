package ontology

import (
	"strings"
	"testing"
)

func TestIndexNameStaysWithinIdentifierLimit(t *testing.T) {
	long := strings.Repeat("x", 60)
	got := indexName("uq", "ont_"+long, long)
	if len(got) > maxIdentLen {
		t.Fatalf("index name length = %d, want <= %d: %s", len(got), maxIdentLen, got)
	}
	if !strings.HasPrefix(got, "uq_ont_") {
		t.Fatalf("index name lost its readable prefix: %s", got)
	}

	other := strings.Repeat("y", 60)
	if got == indexName("uq", "ont_"+other, long) {
		t.Fatal("two different long names collapsed to the same index name")
	}
	if got != indexName("uq", "ont_"+long, long) {
		t.Fatal("index name is not deterministic")
	}
}

func TestIndexNameShortIsReadable(t *testing.T) {
	if got := indexName("idx", "ont_widget", "org_id"); got != "idx_ont_widget_org_id" {
		t.Fatalf("indexName = %q", got)
	}
}

func TestQuoteLiteralEscapes(t *testing.T) {
	if got := quoteLiteral("it's"); got != "'it''s'" {
		t.Fatalf("quoteLiteral = %q", got)
	}
}

func TestFieldString(t *testing.T) {
	f := Field{Name: "sku", Type: TypeString}
	if got := f.String(); got != "sku:string" {
		t.Fatalf("Field.String = %q", got)
	}
}

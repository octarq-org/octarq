package ontology_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/ontology"
)

func valueSpec() ontology.Spec {
	return ontology.Spec{
		Slug:  "tickets",
		Label: "Ticket",
		Fields: []ontology.Field{
			{Name: "title", Type: ontology.TypeString, Required: true},
			{Name: "seats", Type: ontology.TypeInteger},
			{Name: "score", Type: ontology.TypeNumber},
			{Name: "closed", Type: ontology.TypeBoolean},
			{Name: "due_at", Type: ontology.TypeTimestamp},
			{Name: "owner_id", Type: ontology.TypeUUID},
			{Name: "extra", Type: ontology.TypeJSON},
		},
		Operations: ontology.AllowAllOperations(),
	}
}

func TestDecodeValuesCoercesTypes(t *testing.T) {
	got, err := ontology.DecodeValues(valueSpec(), []byte(`{
		"title":"printer on fire",
		"seats":3,
		"score":1.5,
		"closed":true,
		"due_at":"2026-11-13T08:00:00Z",
		"owner_id":"9f9b3d1e-2c4a-4b4e-8a1f-3e2f6c7b8d90",
		"extra":{"a":[1,2]}
	}`))
	if err != nil {
		t.Fatalf("DecodeValues: %v", err)
	}
	if got["seats"] != int64(3) {
		t.Fatalf("seats = %T(%v), want int64(3)", got["seats"], got["seats"])
	}
	if got["score"] != 1.5 {
		t.Fatalf("score = %v, want 1.5", got["score"])
	}
	if got["closed"] != true {
		t.Fatalf("closed = %v, want true", got["closed"])
	}
	if _, ok := got["due_at"].(time.Time); !ok {
		t.Fatalf("due_at = %T, want time.Time", got["due_at"])
	}
	if got["extra"] != `{"a":[1,2]}` {
		t.Fatalf("extra = %v, want the re-marshalled JSON string", got["extra"])
	}
}

func TestDecodeValuesRefusesMissingRequired(t *testing.T) {
	_, err := ontology.DecodeValues(valueSpec(), []byte(`{"seats":1}`))
	assertIssues(t, err, ontology.CodeValueRequired)
}

func TestDecodeValuesRefusesUnknownKey(t *testing.T) {
	_, err := ontology.DecodeValues(valueSpec(), []byte(`{"title":"x","seat":2}`))
	assertIssues(t, err, ontology.CodeValueUnknown)
}

func TestDecodeValuesRefusesReservedKey(t *testing.T) {
	_, err := ontology.DecodeValues(valueSpec(), []byte(`{"title":"x","org_id":2}`))
	assertIssues(t, err, ontology.CodeValueReserved)
}

func TestDecodeValuesRefusesWrongTypes(t *testing.T) {
	for name, body := range map[string]string{
		"integer with fraction":  `{"title":"x","seats":2.5}`,
		"integer as string":      `{"title":"x","seats":"two"}`,
		"boolean as string":      `{"title":"x","closed":"yes"}`,
		"timestamp without zone": `{"title":"x","due_at":"2026-11-13 08:00:00"}`,
		"uuid malformed":         `{"title":"x","owner_id":"not-a-uuid"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ontology.DecodeValues(valueSpec(), []byte(body)); err == nil {
				t.Fatalf("expected an error for %s", body)
			}
		})
	}
}

func TestDecodePatchAllowsOmittedRequired(t *testing.T) {
	got, err := ontology.DecodePatch(valueSpec(), []byte(`{"seats":9}`))
	if err != nil {
		t.Fatalf("DecodePatch: %v", err)
	}
	if _, present := got["title"]; present {
		t.Fatal("an omitted field must not appear in the patch")
	}
	if got["seats"] != int64(9) {
		t.Fatalf("seats = %v, want 9", got["seats"])
	}
}

func TestFromRowRendersColumns(t *testing.T) {
	row := map[string]any{
		"id":         "t1",
		"org_id":     int64(7),
		"created_at": time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC),
		"updated_at": "2026-11-13T09:00:00Z",
		"title":      "printer on fire",
		"seats":      int64(3),
		"closed":     int64(1),
		"due_at":     time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC),
		"extra":      []byte(`{"a":1}`),
	}
	got := ontology.FromRow(valueSpec(), row)
	if got["id"] != "t1" {
		t.Fatalf("id = %v", got["id"])
	}
	if _, exposed := got["org_id"]; exposed {
		t.Fatal("org_id must not be echoed back")
	}
	if got["created_at"] != "2026-11-13T08:00:00Z" {
		t.Fatalf("created_at = %v", got["created_at"])
	}
	if got["closed"] != true {
		t.Fatalf("closed = %v, want true from an integer column", got["closed"])
	}
	if got["due_at"] != "2026-11-14T00:00:00Z" {
		t.Fatalf("due_at = %v", got["due_at"])
	}
	if string(got["extra"].(json.RawMessage)) != `{"a":1}` {
		t.Fatalf("extra = %v, want raw JSON", got["extra"])
	}
}

package ontology

import (
	"context"
	"errors"
	"testing"

	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestIdentity(t *testing.T) {
	p := New()
	if p.Name() != "ontology" {
		t.Fatalf("Name = %q", p.Name())
	}
	info := p.Describe()
	if !info.Core || !info.EnabledByDefault || info.Icon != "database" {
		t.Fatalf("Describe = %+v, want a Core plugin with the database icon", info)
	}
	if models := p.Models(); len(models) != 1 {
		t.Fatalf("Models = %v, want the bookkeeping table", models)
	}
}

func TestMountRefusesANilContext(t *testing.T) {
	p := New()
	p.Mount(nil, nil)
	if err := p.Validate(context.Background(), nil); err == nil {
		t.Fatal("Validate = nil, want the missing context refused")
	}
}

func TestFieldHelpers(t *testing.T) {
	plain := core.Field{Name: "notes", Type: core.TypeString}
	if got := fieldLabel(plain); got != "notes" {
		t.Fatalf("fieldLabel = %q, want the field name", got)
	}
	if got := fieldWidget(plain); got != core.WidgetText {
		t.Fatalf("fieldWidget = %q, want text", got)
	}
	if got := fieldWidget(core.Field{Name: "notes", Type: core.TypeText}); got != core.WidgetTextarea {
		t.Fatalf("fieldWidget(text) = %q, want textarea", got)
	}
	if got := fieldWidget(core.Field{Name: "done", Type: core.TypeBoolean}); got != core.WidgetCheckbox {
		t.Fatalf("fieldWidget(boolean) = %q, want checkbox", got)
	}
	if got := fieldWidget(core.Field{Name: "due", Type: core.TypeTimestamp}); got != core.WidgetDatetime {
		t.Fatalf("fieldWidget(timestamp) = %q, want datetime", got)
	}
	if got := fieldWidget(core.Field{Name: "seats", Type: core.TypeInteger}); got != core.WidgetNumber {
		t.Fatalf("fieldWidget(integer) = %q, want number", got)
	}
	if got := fieldWidget(core.Field{Name: "extra", Type: core.TypeJSON}); got != core.WidgetJSON {
		t.Fatalf("fieldWidget(json) = %q, want json", got)
	}

	labelled := core.Field{Name: "notes", Label: "Notes"}
	if got := pluralLabel(core.Spec{Slug: "notes", Label: "Note"}); got != "Note" {
		t.Fatalf("pluralLabel = %q, want the label as written", got)
	}
	if got := pluralLabel(core.Spec{Slug: "notes", Label: "Note", PluralLabel: "Notebook"}); got != "Notebook" {
		t.Fatalf("pluralLabel = %q, want the declared plural", got)
	}
	if got := fieldLabel(labelled); got != "Notes" {
		t.Fatalf("fieldLabel = %q, want the declared label", got)
	}

	if got := iconFor(core.Spec{Slug: "tickets"}); got != defaultIcon {
		t.Fatalf("iconFor = %q, want the default", got)
	}
	if got := iconFor(core.Spec{Slug: "tickets", Icon: "ticket"}); got != "ticket" {
		t.Fatalf("iconFor = %q, want the declared icon", got)
	}
}

func TestOrderClause(t *testing.T) {
	if got := orderClause(nil); got != core.ColumnCreatedAt+" DESC" {
		t.Fatalf("orderClause(nil) = %q, want newest first", got)
	}
	if got := orderClause([]string{"title", "-priority"}); got != "title ASC, priority DESC" {
		t.Fatalf("orderClause = %q", got)
	}
}

func TestArgIntReadsEveryShapeTheSdkSends(t *testing.T) {
	cases := []struct {
		name string
		in   toolArgs
		want int
	}{
		{"float64", toolArgs{"limit": float64(7)}, 7},
		{"int", toolArgs{"limit": 7}, 7},
		{"int64", toolArgs{"limit": int64(7)}, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := argInt(tc.in, "limit")
			if err != nil || got != tc.want {
				t.Fatalf("argInt = %d, %v; want %d", got, err, tc.want)
			}
		})
	}

	for _, bad := range []toolArgs{
		{},
		{"limit": "7"},
		{"limit": 1.5},
		{"limit": float64(1) * 1e18},
	} {
		if _, err := argInt(bad, "limit"); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("argInt(%v) = %v, want a refusal", bad, err)
		}
	}
}

func TestTypedDefaultAndJSONType(t *testing.T) {
	text := func(s string) *string { return &s }

	if got, ok := typedDefault(core.Field{Type: core.TypeInteger, Default: text("2")}); !ok || got != int64(2) {
		t.Fatalf("typedDefault(integer) = %v, %v", got, ok)
	}
	if got, ok := typedDefault(core.Field{Type: core.TypeNumber, Default: text("1.5")}); !ok || got != 1.5 {
		t.Fatalf("typedDefault(number) = %v, %v", got, ok)
	}
	if got, ok := typedDefault(core.Field{Type: core.TypeBoolean, Default: text("true")}); !ok || got != true {
		t.Fatalf("typedDefault(boolean) = %v, %v", got, ok)
	}
	if got, ok := typedDefault(core.Field{Type: core.TypeString, Default: text("open")}); !ok || got != "open" {
		t.Fatalf("typedDefault(string) = %v, %v", got, ok)
	}
	if _, ok := typedDefault(core.Field{Type: core.TypeString}); ok {
		t.Fatal("typedDefault(no default) = ok, want false")
	}
	if _, ok := typedDefault(core.Field{Type: core.TypeInteger, Default: text("two")}); ok {
		t.Fatal("typedDefault(unparsable) = ok, want false")
	}

	for _, tc := range []struct {
		typ  core.FieldType
		want string
	}{
		{core.TypeInteger, "integer"},
		{core.TypeNumber, "number"},
		{core.TypeBoolean, "boolean"},
		{core.TypeJSON, ""},
		{core.TypeString, "string"},
		{core.TypeUUID, "string"},
	} {
		if got := jsonType(tc.typ); got != tc.want {
			t.Fatalf("jsonType(%q) = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

func TestAsFailureMapsEveryRefusal(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{ErrNotFound, 404, "NOT_FOUND"},
		{ErrNoTenantScope, 401, "UNAUTHENTICATED"},
		{ErrNoDatabase, 503, "DATABASE_UNAVAILABLE"},
		{ErrNoRole, 403, "FORBIDDEN"},
		{ErrNoUserScope, 403, "FORBIDDEN"},
		{ErrForbidden, 403, "FORBIDDEN"},
		{ErrUnknownSort, 422, "UNKNOWN_SORT"},
		{ErrInvalidArgument, 422, "INVALID_ARGUMENT"},
		{core.ErrBookkeepingMissing, 503, "SCHEMA_NOT_APPLIED"},
		{errors.New("driver exploded"), 500, "INTERNAL"},
	}
	for _, tc := range cases {
		f := asFailure(tc.err)
		if f.status != tc.status || f.code != tc.code {
			t.Fatalf("asFailure(%v) = %d %s, want %d %s", tc.err, f.status, f.code, tc.status, tc.code)
		}
	}
	if asFailure(nil) != nil {
		t.Fatal("asFailure(nil) != nil")
	}
	if got := asFailure(ErrForbidden).Error(); got == "" {
		t.Fatal("failure.Error() = empty")
	}
}

func TestAuditIsANoOpWithoutASink(t *testing.T) {
	t.Setenv(EntitiesEnv, ticketDoc)
	t.Setenv(EntitiesFileEnv, "")

	p := New()
	p.Mount(nil, &plugin.Context{DB: openDB(t)})
	p.audit(context.Background(), nil, "data.create", p.entities[0], "abc")

	New().audit(context.Background(), nil, "data.create", p.entities[0], "abc")
}

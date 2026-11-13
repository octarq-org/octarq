package ontology_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/ontology"
)

func strptr(s string) *string { return &s }

func validSpec() ontology.Spec {
	return ontology.Spec{
		Slug:  "widget",
		Label: "Widget",
		Fields: []ontology.Field{
			{Name: "name", Type: ontology.TypeString, Required: true},
			{Name: "count", Type: ontology.TypeInteger, Indexed: true},
		},
		Operations: ontology.AllowAllOperations(),
	}
}

func codes(err error) []string {
	issues, ok := ontology.IsInvalid(err)
	if !ok {
		return nil
	}
	out := make([]string, len(issues))
	for i, iss := range issues {
		out[i] = iss.Code
	}
	return out
}

func hasCode(err error, code string) bool {
	for _, c := range codes(err) {
		if c == code {
			return true
		}
	}
	return false
}

func TestSlugRules(t *testing.T) {
	for _, ok := range []string{"a", "widget", "widget_2", "a" + strings.Repeat("b", 40)} {
		if !ontology.ValidSlug(ok) {
			t.Errorf("ValidSlug(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{
		"", "Widget", "1widget", "_widget", "widget-2", "a" + strings.Repeat("b", 41),
		"ontology", "settings", "users", "data",
	} {
		if ontology.ValidSlug(bad) {
			t.Errorf("ValidSlug(%q) = true, want false", bad)
		}
	}
}

func TestTableNameAndFieldNames(t *testing.T) {
	if got := ontology.TableName("widget"); got != "ont_widget" {
		t.Errorf("TableName = %q, want ont_widget", got)
	}
	for _, bad := range []string{"id", "org_id", "created_at", "updated_at", "Bad"} {
		if ontology.ValidFieldName(bad) {
			t.Errorf("ValidFieldName(%q) = true, want false", bad)
		}
	}
	if !ontology.ValidFieldName("order_total") {
		t.Error("ValidFieldName(order_total) = false, want true")
	}
}

func TestValidateAcceptsAWellFormedSpec(t *testing.T) {
	if err := ontology.Validate(validSpec()); err != nil {
		t.Fatalf("Validate(validSpec) = %v, want nil", err)
	}
}

func TestValidateRejectsBadFields(t *testing.T) {
	base := func(mut func(*ontology.Spec)) ontology.Spec {
		s := validSpec()
		mut(&s)
		return s
	}

	cases := []struct {
		name string
		spec ontology.Spec
		code string
	}{
		{"bad slug", base(func(s *ontology.Spec) { s.Slug = "Widget!" }), ontology.CodeSlugInvalid},
		{"reserved slug", base(func(s *ontology.Spec) { s.Slug = "settings" }), ontology.CodeSlugReserved},
		// applied_specs maps to ont_applied_specs, the bookkeeping table itself.
		// Declaring it would have the reconciler compare a table against a
		// declaration it wrote about other tables.
		{"bookkeeping slug", base(func(s *ontology.Spec) { s.Slug = "applied_specs" }), ontology.CodeSlugReserved},
		{"empty label", base(func(s *ontology.Spec) { s.Label = "  " }), ontology.CodeLabelRequired},
		{"no fields", base(func(s *ontology.Spec) { s.Fields = nil }), ontology.CodeFieldsEmpty},
		{"reserved column", base(func(s *ontology.Spec) {
			s.Fields = append(s.Fields, ontology.Field{Name: "org_id", Type: ontology.TypeInteger})
		}), ontology.CodeNameReserved},
		{"bad name", base(func(s *ontology.Spec) {
			s.Fields = append(s.Fields, ontology.Field{Name: "Bad Name", Type: ontology.TypeString})
		}), ontology.CodeNameInvalid},
		{"duplicate name", base(func(s *ontology.Spec) {
			s.Fields = append(s.Fields, ontology.Field{Name: "NAME", Type: ontology.TypeString})
		}), ontology.CodeNameInvalid},
		{"duplicate field", base(func(s *ontology.Spec) {
			s.Fields = append(s.Fields, ontology.Field{Name: "Name", Type: ontology.TypeString}, ontology.Field{Name: "name", Type: ontology.TypeString})
		}), ontology.CodeNameDuplicate},
		{"unknown type", base(func(s *ontology.Spec) {
			s.Fields = append(s.Fields, ontology.Field{Name: "blob", Type: "bytes"})
		}), ontology.CodeTypeUnknown},
		{"unknown widget", base(func(s *ontology.Spec) {
			s.Fields[0].Widget = "slider"
		}), ontology.CodeWidgetUnknown},
		{"index on text", base(func(s *ontology.Spec) {
			s.Fields = append(s.Fields, ontology.Field{Name: "body", Type: ontology.TypeText, Unique: true})
		}), ontology.CodeIndexUnsupported},
		{"default on json", base(func(s *ontology.Spec) {
			s.Fields = append(s.Fields, ontology.Field{Name: "meta", Type: ontology.TypeJSON, Default: strptr("{}")})
		}), ontology.CodeDefaultUnsupported},
		{"non-numeric integer default", base(func(s *ontology.Spec) {
			s.Fields[1].Default = strptr("many")
		}), ontology.CodeDefaultInvalid},
		{"non-boolean default", base(func(s *ontology.Spec) {
			s.Fields = append(s.Fields, ontology.Field{Name: "on", Type: ontology.TypeBoolean, Default: strptr("yes")})
		}), ontology.CodeDefaultInvalid},
		{"missing operation", base(func(s *ontology.Spec) {
			delete(s.Operations, ontology.OpDelete)
		}), ontology.CodeACLEmpty},
		{"empty roles", base(func(s *ontology.Spec) {
			s.Operations[ontology.OpGet] = ontology.OperationACL{}
		}), ontology.CodeACLEmpty},
		{"unknown role", base(func(s *ontology.Spec) {
			s.Operations[ontology.OpGet] = ontology.OperationACL{Roles: []string{"root"}}
		}), ontology.CodeRoleUnknown},
		{"unknown operation", base(func(s *ontology.Spec) {
			s.Operations["archive"] = ontology.OperationACL{Roles: []string{"owner"}}
		}), ontology.CodeACLEmpty},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ontology.Validate(tc.spec)
			if err == nil {
				t.Fatalf("Validate = nil, want %s", tc.code)
			}
			if !hasCode(err, tc.code) {
				t.Fatalf("codes = %v, want %s", codes(err), tc.code)
			}
			if _, ok := ontology.IsInvalid(err); !ok {
				t.Fatalf("IsInvalid(%v) = false", err)
			}
		})
	}
}

func TestValidateReportsEveryProblemAtOnce(t *testing.T) {
	spec := ontology.Spec{Slug: "Bad!", Fields: nil, Operations: nil}
	err := ontology.Validate(spec)
	got := codes(err)
	for _, want := range []string{ontology.CodeSlugInvalid, ontology.CodeLabelRequired, ontology.CodeFieldsEmpty, ontology.CodeACLEmpty} {
		if !contains(got, want) {
			t.Errorf("codes = %v, missing %s", got, want)
		}
	}
}

func TestErrorsIsAndEmpty(t *testing.T) {
	var empty ontology.Errors
	if empty.Error() != "" {
		t.Errorf("empty Error() = %q", empty.Error())
	}
	issues, ok := ontology.IsInvalid(empty)
	if !ok || issues != nil {
		t.Errorf("IsInvalid(empty) = %v, %v; want nil, true", issues, ok)
	}
	if _, ok := ontology.IsInvalid(errors.New("boom")); ok {
		t.Error("IsInvalid(plain error) = true, want false")
	}
	if got := (ontology.Errors{{Code: "X", Path: "p", Message: "m"}}).Error(); !strings.Contains(got, "X at p: m") {
		t.Errorf("Error() = %q", got)
	}
}

func TestValidateTransition(t *testing.T) {
	old := validSpec()

	t.Run("slug is immutable", func(t *testing.T) {
		next := validSpec()
		next.Slug = "gadget"
		if err := ontology.ValidateTransition(old, next, 0); !hasCode(err, ontology.CodeSlugImmutable) {
			t.Fatalf("err = %v, want SLUG_IMMUTABLE", err)
		}
	})

	t.Run("drop column fails even when empty", func(t *testing.T) {
		next := validSpec()
		next.Fields = next.Fields[:1]
		if err := ontology.ValidateTransition(old, next, 0); !hasCode(err, ontology.CodeColumnDropUnsupported) {
			t.Fatalf("err = %v, want COLUMN_DROP_UNSUPPORTED", err)
		}
	})

	t.Run("drop column with rows fails", func(t *testing.T) {
		next := validSpec()
		next.Fields = next.Fields[:1]
		if err := ontology.ValidateTransition(old, next, 3); !hasCode(err, ontology.CodeColumnDropUnsupported) {
			t.Fatalf("err = %v, want COLUMN_DROP_UNSUPPORTED", err)
		}
	})

	t.Run("type change fails even when empty", func(t *testing.T) {
		next := validSpec()
		next.Fields[1].Type = ontology.TypeString
		if err := ontology.ValidateTransition(old, next, 0); !hasCode(err, ontology.CodeTypeChangeUnsupported) {
			t.Fatalf("err = %v, want TYPE_CHANGE_UNSUPPORTED", err)
		}
	})

	t.Run("required column needs a default on a non-empty table", func(t *testing.T) {
		next := validSpec()
		next.Fields = append(next.Fields, ontology.Field{Name: "sku", Type: ontology.TypeString, Required: true})
		if err := ontology.ValidateTransition(old, next, 5); !hasCode(err, ontology.CodeRequiredFieldNonEmpty) {
			t.Fatalf("err = %v, want REQUIRED_FIELD_NONEMPTY", err)
		}
		next.Fields[len(next.Fields)-1].Default = strptr("n/a")
		if err := ontology.ValidateTransition(old, next, 5); err != nil {
			t.Fatalf("err = %v, want nil once a default is present", err)
		}
	})

	t.Run("optional column is allowed on a non-empty table", func(t *testing.T) {
		next := validSpec()
		next.Fields = append(next.Fields, ontology.Field{Name: "note", Type: ontology.TypeText})
		if err := ontology.ValidateTransition(old, next, 5); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

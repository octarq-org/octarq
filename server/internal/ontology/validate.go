package ontology

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Issue codes. Each one is stable: callers branch on it, and operators see it in
// the error payload.
const (
	CodeSlugInvalid           = "SLUG_INVALID"
	CodeSlugReserved          = "SLUG_RESERVED"
	CodeSlugImmutable         = "SLUG_IMMUTABLE"
	CodeLabelRequired         = "LABEL_REQUIRED"
	CodeFieldsEmpty           = "FIELDS_EMPTY"
	CodeNameInvalid           = "NAME_INVALID"
	CodeNameReserved          = "NAME_RESERVED"
	CodeNameDuplicate         = "NAME_DUPLICATE"
	CodeTypeUnknown           = "TYPE_UNKNOWN"
	CodeWidgetUnknown         = "WIDGET_UNKNOWN"
	CodeRequiredFieldNonEmpty = "REQUIRED_FIELD_NONEMPTY"
	CodeTypeChangeUnsupported = "TYPE_CHANGE_UNSUPPORTED"
	CodeColumnDropNonEmpty    = "COLUMN_DROP_NONEMPTY"
	CodeACLEmpty              = "ACL_EMPTY"
	CodeRoleUnknown           = "ROLE_UNKNOWN"
	CodeIndexUnsupported      = "INDEX_UNSUPPORTED"
	CodeDefaultUnsupported    = "DEFAULT_UNSUPPORTED"
	CodeDefaultInvalid        = "DEFAULT_INVALID"
)

// validDefault reports whether a literal default parses as its declared type.
// Numeric and boolean defaults render unquoted in DDL, so a value that does not
// parse would produce a statement the database either rejects or coerces
// silently — both fail-closed violations.
func validDefault(t FieldType, raw string) bool {
	s := strings.TrimSpace(raw)
	switch t {
	case TypeInteger:
		_, err := strconv.ParseInt(s, 10, 64)
		return err == nil
	case TypeNumber:
		_, err := strconv.ParseFloat(s, 64)
		return err == nil
	case TypeBoolean:
		return s == "true" || s == "false"
	default:
		return true
	}
}

// indexableTypes and defaultableTypes are the types that stay portable across
// all three dialects. MySQL cannot index a TEXT or JSON column without a prefix
// length, and cannot give one a literal default; refusing the declaration is
// cleaner than emitting SQL that fails only on MySQL.
var (
	indexableTypes   = map[FieldType]bool{TypeString: true, TypeInteger: true, TypeBoolean: true, TypeNumber: true, TypeTimestamp: true, TypeUUID: true}
	defaultableTypes = map[FieldType]bool{TypeString: true, TypeInteger: true, TypeBoolean: true, TypeNumber: true, TypeTimestamp: true, TypeUUID: true}
)

// Issue is one reason a declaration was refused. Path points at the offending
// part of the document, so a form can highlight the field.
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Errors is a non-empty list of issues, and satisfies error.
type Errors []Issue

func (e Errors) Error() string {
	if len(e) == 0 {
		return ""
	}
	parts := make([]string, len(e))
	for i, iss := range e {
		parts[i] = iss.Code + " at " + iss.Path + ": " + iss.Message
	}
	return "invalid entity: " + strings.Join(parts, "; ")
}

// IsInvalid reports whether err is a validation failure, and returns its issues.
func IsInvalid(err error) (Errors, bool) {
	var e Errors
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

func issue(code, path, format string, args ...any) Issue {
	return Issue{Code: code, Path: path, Message: fmt.Sprintf(format, args...)}
}

func normalize(errs Errors) error {
	if len(errs) == 0 {
		return nil
	}
	sort.SliceStable(errs, func(i, j int) bool {
		if errs[i].Path != errs[j].Path {
			return errs[i].Path < errs[j].Path
		}
		return errs[i].Code < errs[j].Code
	})
	return errs
}

// Validate checks a declaration against everything that can be decided without
// looking at the database.
func Validate(spec Spec) error {
	var errs Errors

	switch {
	case !SlugPattern.MatchString(spec.Slug):
		errs = append(errs, issue(CodeSlugInvalid, "slug",
			"must match %s", SlugPattern))
	case reservedSlugs[spec.Slug]:
		errs = append(errs, issue(CodeSlugReserved, "slug",
			"%q is reserved by the platform", spec.Slug))
	}

	if strings.TrimSpace(spec.Label) == "" {
		errs = append(errs, issue(CodeLabelRequired, "label", "label cannot be empty"))
	}

	if len(spec.Fields) == 0 {
		errs = append(errs, issue(CodeFieldsEmpty, "fields", "an entity needs at least one field"))
	}

	seen := make(map[string]bool, len(spec.Fields))
	for i, f := range spec.Fields {
		path := fmt.Sprintf("fields[%d]", i)
		if !ValidFieldName(f.Name) {
			code := CodeNameInvalid
			if ReservedColumnNames[f.Name] {
				code = CodeNameReserved
			}
			errs = append(errs, issue(code, path+".name", "%q is not an allowed field name", f.Name))
		}
		key := strings.ToLower(f.Name)
		if f.Name != "" && seen[key] {
			errs = append(errs, issue(CodeNameDuplicate, path+".name", "%q is declared twice", f.Name))
		}
		seen[key] = true
		if !validType(f.Type) {
			errs = append(errs, issue(CodeTypeUnknown, path+".type", "%q is not a known type", f.Type))
		}
		if !validWidget(f.Widget) {
			errs = append(errs, issue(CodeWidgetUnknown, path+".widget", "%q is not a known widget", f.Widget))
		}
		if (f.Unique || f.Indexed) && !indexableTypes[f.Type] {
			errs = append(errs, issue(CodeIndexUnsupported, path+".unique",
				"%s fields cannot be indexed; use string", f.Type))
		}
		if f.Default != nil && !defaultableTypes[f.Type] {
			errs = append(errs, issue(CodeDefaultUnsupported, path+".default",
				"%s fields cannot carry a literal default", f.Type))
		}
		if f.Default != nil && defaultableTypes[f.Type] && !validDefault(f.Type, *f.Default) {
			errs = append(errs, issue(CodeDefaultInvalid, path+".default",
				"%q is not a valid %s default", *f.Default, f.Type))
		}
	}

	for _, op := range Operations {
		path := "operations." + string(op)
		acl, ok := spec.Operations[op]
		if !ok {
			errs = append(errs, issue(CodeACLEmpty, path, "every operation needs an ACL entry"))
			continue
		}
		if len(acl.Roles) == 0 {
			errs = append(errs, issue(CodeACLEmpty, path+".roles", "at least one role is required"))
		}
		for _, r := range acl.Roles {
			if !validRole(r) {
				errs = append(errs, issue(CodeRoleUnknown, path+".roles", "%q is not a known role", r))
			}
		}
	}
	for op := range spec.Operations {
		known := false
		for _, want := range Operations {
			if op == want {
				known = true
				break
			}
		}
		if !known {
			errs = append(errs, issue(CodeACLEmpty, "operations."+string(op), "%q is not a known operation", op))
		}
	}

	return normalize(errs)
}

// ValidateTransition checks a change against the stored declaration and the
// number of rows already in the entity's table. It refuses anything the DDL
// layer cannot apply without losing data.
func ValidateTransition(old, next Spec, rows int64) error {
	var errs Errors

	if old.Slug != next.Slug {
		errs = append(errs, issue(CodeSlugImmutable, "slug",
			"cannot change slug from %q to %q", old.Slug, next.Slug))
	}

	oldFields := make(map[string]Field, len(old.Fields))
	for _, f := range old.Fields {
		oldFields[f.Name] = f
	}
	nextFields := make(map[string]Field, len(next.Fields))
	for _, f := range next.Fields {
		nextFields[f.Name] = f
	}

	for _, f := range old.Fields {
		n, kept := nextFields[f.Name]
		if !kept {
			if rows > 0 {
				errs = append(errs, issue(CodeColumnDropNonEmpty, "fields."+f.Name,
					"cannot drop %q while the table holds %d rows", f.Name, rows))
			}
			continue
		}
		if n.Type != f.Type {
			errs = append(errs, issue(CodeTypeChangeUnsupported, "fields."+n.Name+".type",
				"cannot change %q from %s to %s", n.Name, f.Type, n.Type))
		}
	}

	for _, f := range next.Fields {
		if _, existed := oldFields[f.Name]; existed {
			continue
		}
		if rows > 0 && f.Required && f.Default == nil {
			errs = append(errs, issue(CodeRequiredFieldNonEmpty, "fields."+f.Name,
				"required field %q needs a default value while the table holds %d rows", f.Name, rows))
		}
	}

	return normalize(errs)
}

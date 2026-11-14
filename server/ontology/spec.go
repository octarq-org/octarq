// Package ontology holds the declarative entity model behind Octarq's
// data-driven CRUD surface.
//
// An operator declares an entity as data: a slug, a closed set of fields and the
// operations allowed on it. The platform turns that declaration into a table, a
// REST surface and MCP tools. Nothing here touches a database; this package is
// the pure half — the spec, its validation, and the SQL builders that apply it.
//
// The rules are fail-closed. A declaration that cannot be applied exactly as
// written is rejected with a code the caller can show the operator; there is no
// "best effort" path.
package ontology

import (
	"fmt"
	"regexp"
	"strings"
)

// FieldType is the closed set of column types an entity may declare. The set is
// deliberately small: every member has an exact equivalent in SQLite, Postgres
// and MySQL, so the generated DDL is predictable. Types that need per-dialect
// semantics (decimal precision, arrays, enums) are absent by design.
type FieldType string

const (
	TypeString    FieldType = "string"
	TypeText      FieldType = "text"
	TypeInteger   FieldType = "integer"
	TypeBoolean   FieldType = "boolean"
	TypeNumber    FieldType = "number"
	TypeTimestamp FieldType = "timestamp"
	TypeUUID      FieldType = "uuid"
	TypeJSON      FieldType = "json"
)

// FieldTypes lists the accepted types, in documentation order.
var FieldTypes = []FieldType{
	TypeString, TypeText, TypeInteger, TypeBoolean,
	TypeNumber, TypeTimestamp, TypeUUID, TypeJSON,
}

// Widget is how the generic admin form renders a field. It is presentation only;
// it never changes storage.
type Widget string

const (
	WidgetText     Widget = "text"
	WidgetTextarea Widget = "textarea"
	WidgetNumber   Widget = "number"
	WidgetCheckbox Widget = "checkbox"
	WidgetDatetime Widget = "datetime"
	WidgetJSON     Widget = "json"
)

// Widgets lists the accepted widgets.
var Widgets = []Widget{
	WidgetText, WidgetTextarea, WidgetNumber, WidgetCheckbox, WidgetDatetime, WidgetJSON,
}

// Operation is one action the generated API exposes for an entity.
type Operation string

const (
	OpList   Operation = "list"
	OpGet    Operation = "get"
	OpCreate Operation = "create"
	OpUpdate Operation = "update"
	OpDelete Operation = "delete"
)

// Operations lists every operation an entity must declare.
var Operations = []Operation{OpList, OpGet, OpCreate, OpUpdate, OpDelete}

// Role names an operation may grant. These are the platform's built-in roles.
var Roles = []string{"owner", "admin", "member"}

// Field is one column of a declared entity.
type Field struct {
	Name       string    `json:"name"`
	Type       FieldType `json:"type"`
	Required   bool      `json:"required,omitempty"`
	Unique     bool      `json:"unique,omitempty"`
	Indexed    bool      `json:"indexed,omitempty"`
	Default    *string   `json:"default,omitempty"`
	Label      string    `json:"label,omitempty"`
	Widget     Widget    `json:"widget,omitempty"`
	ListColumn bool      `json:"listColumn,omitempty"`
}

// OperationACL grants one operation to a set of roles.
type OperationACL struct {
	Roles           []string `json:"roles"`
	ExposeMCP       bool     `json:"exposeMcp,omitempty"`
	Tier            string   `json:"tier,omitempty"`
	RequireApproval bool     `json:"requireApproval,omitempty"`
}

// License tiers an operation can require. They are the self-hosted ladder, where
// elite covers pro; an empty tier is free.
const (
	TierPro   = "pro"
	TierElite = "elite"
)

// ScopeKind is the dimension a table is partitioned by. It decides which columns
// the platform adds, which columns every index starts with, and which value the
// tenant handle binds when it reads.
type ScopeKind string

const (
	// ScopeOrg is the default: a row belongs to the workspace, and every member
	// of the workspace sees it.
	ScopeOrg ScopeKind = "org"
	// ScopeUser partitions the table by the member who owns the row. A row is
	// written by one member and read by that member only; the platform adds the
	// column and binds the value, so no caller filters by owner by hand.
	ScopeUser ScopeKind = "user"
)

// ScopeKinds lists the accepted scopes, in documentation order.
var ScopeKinds = []ScopeKind{ScopeOrg, ScopeUser}

// Spec is one entity declaration.
type Spec struct {
	Slug        string                     `json:"slug"`
	Label       string                     `json:"label"`
	PluralLabel string                     `json:"pluralLabel,omitempty"`
	Icon        string                     `json:"icon,omitempty"`
	Scope       ScopeKind                  `json:"scope,omitempty"`
	Fields      []Field                    `json:"fields"`
	Operations  map[Operation]OperationACL `json:"operations"`
}

// ScopeOf returns the declared scope. An empty value means ScopeOrg: a
// declaration that says nothing about scope is a workspace-wide table, which is
// what every declaration written before scopes existed means.
func ScopeOf(spec Spec) ScopeKind {
	if spec.Scope == "" {
		return ScopeOrg
	}
	return spec.Scope
}

// validScope reports whether s is one of ScopeKinds, or empty for the default.
func validScope(s ScopeKind) bool {
	if s == "" {
		return true
	}
	for _, known := range ScopeKinds {
		if s == known {
			return true
		}
	}
	return false
}

// scopeNames renders the accepted scopes for an error message.
func scopeNames() string {
	names := make([]string, len(ScopeKinds))
	for i, s := range ScopeKinds {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

// SlugPattern is the only shape an entity slug may take. The table name and the
// API path both derive from it, so a slug that needed escaping cannot exist.
var SlugPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)

// TablePrefix namespaces every generated table, so a declaration can never
// collide with a core table or a `tenant_` view by accident.
const TablePrefix = "ont_"

// reservedSlugs are refused even though they match SlugPattern: each one is
// either a core table name or a path segment the platform already owns.
var reservedSlugs = map[string]bool{
	"applied_specs": true,
	"auth":          true, "data": true, "help": true, "instance": true,
	"mcp": true, "menus": true, "ontology": true, "plugins": true,
	"portal": true, "settings": true, "schema": true, "tokens": true,
	"users": true, "webhooks": true,
}

// ReservedColumnNames cannot be declared as fields: the platform owns them.
// user_id is reserved whether or not the entity is user-scoped, so adding a
// scope later never collides with a field the operator already declared.
var ReservedColumnNames = map[string]bool{
	"id": true, "org_id": true, "user_id": true,
	"created_at": true, "updated_at": true,
}

// TableName returns the physical table for a slug.
func TableName(slug string) string { return TablePrefix + slug }

// AllowAllOperations returns an ACL map that grants every operation to every
// built-in role. It is the value a fresh declaration starts from.
func AllowAllOperations() map[Operation]OperationACL {
	out := make(map[Operation]OperationACL, len(Operations))
	for _, op := range Operations {
		out[op] = OperationACL{Roles: append([]string(nil), Roles...)}
	}
	return out
}

// ValidSlug reports whether s may be used as an entity slug.
func ValidSlug(s string) bool { return SlugPattern.MatchString(s) && !reservedSlugs[s] }

// ValidFieldName reports whether s may be used as a field name.
func ValidFieldName(s string) bool {
	return SlugPattern.MatchString(s) && !ReservedColumnNames[strings.ToLower(s)]
}

// validType reports whether t is one of FieldTypes.
func validType(t FieldType) bool {
	for _, known := range FieldTypes {
		if t == known {
			return true
		}
	}
	return false
}

// validWidget reports whether w is empty or one of Widgets.
func validWidget(w Widget) bool {
	if w == "" {
		return true
	}
	for _, known := range Widgets {
		if w == known {
			return true
		}
	}
	return false
}

// validRole reports whether r is one of Roles.
func validRole(r string) bool {
	for _, known := range Roles {
		if r == known {
			return true
		}
	}
	return false
}

// validTier reports whether tier is one the license ladder knows, or free.
func validTier(tier string) bool {
	switch tier {
	case "", TierPro, TierElite:
		return true
	default:
		return false
	}
}

// FieldByType maps a declared type to a Go-free zero value used when a column is
// added to a table that already has rows.
func (f Field) sqlDefault() (string, bool) {
	if f.Default == nil {
		return "", false
	}
	return *f.Default, true
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// String renders a field for logs and error messages.
func (f Field) String() string {
	return fmt.Sprintf("%s:%s", f.Name, f.Type)
}

package ontology

import (
	"fmt"
	"regexp"
	"strings"
)

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

var FieldTypes = []FieldType{
	TypeString, TypeText, TypeInteger, TypeBoolean,
	TypeNumber, TypeTimestamp, TypeUUID, TypeJSON,
}

type Widget string

const (
	WidgetText     Widget = "text"
	WidgetTextarea Widget = "textarea"
	WidgetNumber   Widget = "number"
	WidgetCheckbox Widget = "checkbox"
	WidgetDatetime Widget = "datetime"
	WidgetJSON     Widget = "json"
)

var Widgets = []Widget{
	WidgetText, WidgetTextarea, WidgetNumber, WidgetCheckbox, WidgetDatetime, WidgetJSON,
}

type Operation string

const (
	OpList   Operation = "list"
	OpGet    Operation = "get"
	OpCreate Operation = "create"
	OpUpdate Operation = "update"
	OpDelete Operation = "delete"
)

var Operations = []Operation{OpList, OpGet, OpCreate, OpUpdate, OpDelete}

var Roles = []string{"owner", "admin", "member"}

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

type OperationACL struct {
	Roles           []string `json:"roles"`
	ExposeMCP       bool     `json:"exposeMcp,omitempty"`
	Tier            string   `json:"tier,omitempty"`
	RequireApproval bool     `json:"requireApproval,omitempty"`
}

const (
	TierPro   = "pro"
	TierElite = "elite"
)

type ScopeKind string

const (
	ScopeOrg  ScopeKind = "org"
	ScopeUser ScopeKind = "user"
)

var ScopeKinds = []ScopeKind{ScopeOrg, ScopeUser}

type Spec struct {
	Slug        string                     `json:"slug"`
	Label       string                     `json:"label"`
	PluralLabel string                     `json:"pluralLabel,omitempty"`
	Icon        string                     `json:"icon,omitempty"`
	Scope       ScopeKind                  `json:"scope,omitempty"`
	Owner       string                     `json:"-"`
	Fields      []Field                    `json:"fields"`
	Operations  map[Operation]OperationACL `json:"operations"`
}

func ScopeOf(spec Spec) ScopeKind {
	if spec.Scope == "" {
		return ScopeOrg
	}
	return spec.Scope
}

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

func scopeNames() string {
	names := make([]string, len(ScopeKinds))
	for i, s := range ScopeKinds {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

var SlugPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)

const TablePrefix = "ont_"

var reservedSlugs = map[string]bool{
	"applied_specs": true,
	"auth":          true, "data": true, "help": true, "instance": true,
	"mcp": true, "menus": true, "ontology": true, "plugins": true,
	"portal": true, "settings": true, "schema": true, "tokens": true,
	"users": true, "webhooks": true,
}

var ReservedColumnNames = map[string]bool{
	"id": true, "org_id": true, "user_id": true,
	"created_at": true, "updated_at": true,
}

func TableName(slug string) string { return TablePrefix + slug }

func AllowAllOperations() map[Operation]OperationACL {
	out := make(map[Operation]OperationACL, len(Operations))
	for _, op := range Operations {
		out[op] = OperationACL{Roles: append([]string(nil), Roles...)}
	}
	return out
}

func ValidSlug(s string) bool { return SlugPattern.MatchString(s) && !reservedSlugs[s] }

func ValidFieldName(s string) bool {
	return SlugPattern.MatchString(s) && !ReservedColumnNames[strings.ToLower(s)]
}

func validType(t FieldType) bool {
	for _, known := range FieldTypes {
		if t == known {
			return true
		}
	}
	return false
}

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

func validRole(r string) bool {
	for _, known := range Roles {
		if r == known {
			return true
		}
	}
	return false
}

func validTier(tier string) bool {
	switch tier {
	case "", TierPro, TierElite:
		return true
	default:
		return false
	}
}

func (f Field) sqlDefault() (string, bool) {
	if f.Default == nil {
		return "", false
	}
	return *f.Default, true
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func (f Field) String() string {
	return fmt.Sprintf("%s:%s", f.Name, f.Type)
}

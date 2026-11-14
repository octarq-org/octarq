package ontology

import (
	core "github.com/octarq-org/octarq/server/ontology"
)

// entity is one declared spec together with the values derived from it once at
// load time. Deriving them here keeps the request path free of string building
// and keeps the route, the table and the catalog entry from disagreeing.
type entity struct {
	spec  core.Spec
	table string
}

func newEntity(spec core.Spec) *entity {
	return &entity{spec: spec, table: core.TableName(spec.Slug)}
}

// validate refuses the part of a declaration only this package can judge: the
// icon the sidebar must render. core.Validate has already accepted the rest.
func (e *entity) validate() error { return validateIcon(e.spec) }

// acl returns the declaration for one operation. Load-time validation guarantees
// every operation is present, so a missing entry is a programming error rather
// than a tenant condition.
func (e *entity) acl(op core.Operation) core.OperationACL { return e.spec.Operations[op] }

// sortable reports whether name is a column the list endpoint may sort by.
// Declared fields qualify, and so do the two platform timestamps: they exist on
// every generated table and the default order already uses created_at, so
// refusing them would be a surprise. org_id is excluded — it is the same value
// on every row of a workspace, and id is a UUID.
func (e *entity) sortable(name string) bool {
	if name == core.ColumnCreatedAt || name == core.ColumnUpdatedAt {
		return true
	}
	for _, f := range e.spec.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

// catalogEntry is what GET /api/data serves: the declaration as data, stripped
// of everything the caller cannot use. It is the input the generic admin page
// builds its table and form from, and the contract an agent reads before calling
// a data tool.
type catalogEntry struct {
	Slug        string         `json:"slug"`
	Label       string         `json:"label"`
	PluralLabel string         `json:"pluralLabel"`
	Icon        string         `json:"icon,omitempty"`
	Fields      []catalogField `json:"fields"`
	Operations  []string       `json:"operations"`
}

type catalogField struct {
	Name       string         `json:"name"`
	Type       core.FieldType `json:"type"`
	Label      string         `json:"label"`
	Widget     core.Widget    `json:"widget"`
	Required   bool           `json:"required"`
	Unique     bool           `json:"unique"`
	ListColumn bool           `json:"listColumn"`
}

// catalog renders the entity for a caller who holds role. Operations the caller
// cannot perform are omitted, so the page never draws a button that answers 403.
func (e *entity) catalog(can func(op core.Operation) bool) catalogEntry {
	out := catalogEntry{
		Slug:        e.spec.Slug,
		Label:       e.spec.Label,
		PluralLabel: pluralLabel(e.spec),
		Icon:        e.spec.Icon,
		Fields:      make([]catalogField, 0, len(e.spec.Fields)),
		Operations:  []string{},
	}
	for _, f := range e.spec.Fields {
		out.Fields = append(out.Fields, catalogField{
			Name:       f.Name,
			Type:       f.Type,
			Label:      fieldLabel(f),
			Widget:     fieldWidget(f),
			Required:   f.Required,
			Unique:     f.Unique,
			ListColumn: f.ListColumn,
		})
	}
	for _, op := range core.Operations {
		if can(op) {
			out.Operations = append(out.Operations, string(op))
		}
	}
	return out
}

func fieldLabel(f core.Field) string {
	if f.Label != "" {
		return f.Label
	}
	return f.Name
}

// pluralLabel is the display name for a collection. A declaration that names one
// gets it; otherwise the label is used as-is rather than guessed at, because an
// "+s" that is wrong reads worse than no plural.
func pluralLabel(spec core.Spec) string {
	if spec.PluralLabel != "" {
		return spec.PluralLabel
	}
	return spec.Label
}

// fieldWidget is the declared widget, or the one the type implies. The default
// lives here rather than in the frontend so an older page and a newer
// declaration cannot disagree about how a json field renders.
func fieldWidget(f core.Field) core.Widget {
	if f.Widget != "" {
		return f.Widget
	}
	switch f.Type {
	case core.TypeText:
		return core.WidgetTextarea
	case core.TypeInteger, core.TypeNumber:
		return core.WidgetNumber
	case core.TypeBoolean:
		return core.WidgetCheckbox
	case core.TypeTimestamp:
		return core.WidgetDatetime
	case core.TypeJSON:
		return core.WidgetJSON
	default:
		return core.WidgetText
	}
}

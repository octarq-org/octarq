package ontology

import (
	core "github.com/octarq-org/octarq/server/ontology"
)

type entity struct {
	spec       core.Spec
	table      string
	featureKey string
	active     func(orgID uint, featureKey string) bool
}

func newEntity(spec core.Spec) *entity {
	return &entity{spec: spec, table: core.TableName(spec.Slug)}
}

func (e *entity) owned() bool { return e.spec.Owner != "" }

func (e *entity) featureOff(orgID uint) bool {
	if !e.owned() {
		return false
	}
	if e.active == nil {
		return true
	}
	return !e.active(orgID, e.featureKey)
}

func (e *entity) validate() error { return validateIcon(e.spec) }

func (e *entity) acl(op core.Operation) core.OperationACL { return e.spec.Operations[op] }

func (e *entity) declares(name string) bool {
	for _, f := range e.spec.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

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

type catalogEntry struct {
	Slug        string         `json:"slug"`
	Label       string         `json:"label"`
	PluralLabel string         `json:"pluralLabel"`
	Icon        string         `json:"icon,omitempty"`
	Scope       core.ScopeKind `json:"scope"`
	Tier        string         `json:"tier,omitempty"`
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

func (e *entity) catalog(can func(op core.Operation) bool) catalogEntry {
	out := catalogEntry{
		Slug:        e.spec.Slug,
		Label:       e.spec.Label,
		PluralLabel: pluralLabel(e.spec),
		Icon:        e.spec.Icon,
		Scope:       core.ScopeOf(e.spec),
		Tier:        e.acl(core.OpList).Tier,
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

func pluralLabel(spec core.Spec) string {
	if spec.PluralLabel != "" {
		return spec.PluralLabel
	}
	return spec.Label
}

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

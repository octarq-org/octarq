package notes

import (
	"github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

var entity = ontology.Spec{
	Slug:        "notes",
	Label:       "Note",
	PluralLabel: "Notes",
	Icon:        "scroll-text",
	Scope:       ontology.ScopeOrg,
	Fields: []ontology.Field{
		{
			Name: "title", Type: ontology.TypeString, Required: true, Unique: true,
			Label: "Title", Widget: ontology.WidgetText, ListColumn: true,
		},
		{
			Name: "body", Type: ontology.TypeText,
			Label: "Body", Widget: ontology.WidgetTextarea,
		},
		{
			Name: "pinned", Type: ontology.TypeBoolean,
			Label: "Pinned", Widget: ontology.WidgetCheckbox, ListColumn: true,
		},
	},
	Operations: ontology.AllowAllOperations(),
}

func init() { ontology.MustRegister(Plugin{}.Name(), entity) }

type Plugin struct{}

func (Plugin) Name() string { return "notes" }

func (Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:       "Notes",
		Description: "Workspace notes, with an API and agent tools.",
		Category:    plugin.CategoryUtilities,
		Requires:    []string{"ontology"},
	}
}

func (Plugin) Models() []any { return nil }

func (Plugin) Mount(plugin.Mux, *plugin.Context) {}

var (
	_ plugin.Plugin    = Plugin{}
	_ plugin.Describer = Plugin{}
)

// Package notes is a copy-me example of a plugin whose whole feature is one
// declared entity: a table, a REST surface, an admin page and agent tools, none
// of which the plugin writes any Go for.
//
// It is the other half of ./plugin-hello. That example writes its own route;
// this one shows the part a plugin can stop writing. Copy this file, change the
// spec, and the platform builds the rest.
//
// It is deliberately NOT mounted in server/main.go. The ontology plugin owns the
// routes and the sidebar entry of every declared entity, and it does not consult
// the declaring plugin's feature toggle — so mounting this would put a Notes
// page in every OSS install, and turning "notes" off in Settings → Plugins would
// not remove it. A real module wants its page; an example does not.
package notes

import (
	"github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

// entity is the declaration. It is a package variable rather than a literal
// inside init so a test can validate exactly what gets registered.
var entity = ontology.Spec{
	Slug:        "notes",
	Label:       "Note",
	PluralLabel: "Notes",
	Icon:        "scroll-text",
	// ScopeOrg: every member of the workspace sees the note. ScopeUser would
	// partition the table by the member who wrote the row, and the platform
	// binds that column — no query here filters by owner.
	Scope: ontology.ScopeOrg,
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
	// Every operation for every role, with no tier: the example stays usable
	// without a license. A module that sells its entities sets OperationACL.Tier.
	Operations: ontology.AllowAllOperations(),
}

// init declares the entity before any plugin mounts. It is the only place a
// module can declare: the composition root receives []plugin.Plugin and never
// holds the ontology plugin instance, so there is nothing to call later. The
// registry refuses a declaration that arrives after boot.
func init() { ontology.MustRegister(entity) }

// Plugin is the value a composition root mounts with app.App.Use(notes.Plugin{}).
type Plugin struct{}

// Name is the stable identifier. It is not the entity slug: a plugin may declare
// several entities, and the slug is the one the URL and the table use.
func (Plugin) Name() string { return "notes" }

// Describe names the plugin and requires the ontology plugin. Requires is
// checked at composition, so a build that drops the ontology plugin refuses to
// start instead of shipping a plugin with no surface at all.
func (Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:       "Notes",
		Description: "Workspace notes, with an API and agent tools.",
		Category:    plugin.CategoryUtilities,
		Requires:    []string{"ontology"},
	}
}

// Models is empty: the ontology plugin owns the table. A plugin that also has
// hand-written tables returns their GORM models here as before.
func (Plugin) Models() []any { return nil }

// Mount is empty: the routes are /api/data/notes, registered by the ontology
// plugin. A plugin adds Mount only for endpoints a declaration cannot express.
func (Plugin) Mount(plugin.Mux, *plugin.Context) {}

var (
	_ plugin.Plugin    = Plugin{}
	_ plugin.Describer = Plugin{}
)

package plugin

// Category constants for plugin classification.
const (
	CategoryMarketing      = "marketing"
	CategoryMessaging      = "messaging"
	CategorySecurity       = "security"
	CategoryInfrastructure = "infrastructure"
	CategoryCommerce       = "commerce"
	CategoryAI             = "ai"
	CategoryUtilities      = "utilities"
)

// ValidCategories is the set of allowed category constants for plugins.
var ValidCategories = map[string]bool{
	CategoryMarketing:      true,
	CategoryMessaging:      true,
	CategorySecurity:       true,
	CategoryInfrastructure: true,
	CategoryCommerce:       true,
	CategoryAI:             true,
	CategoryUtilities:      true,
}

// Info is optional presentation/enablement metadata for a plugin. A plugin that
// does not implement Describer is treated as a standalone, user-toggleable
// feature keyed and titled by its Name().
type Info struct {
	// Title is the human label shown in the plugin manager (e.g. "Commerce").
	// Empty falls back to Name(), or, for a group, to the first member that
	// sets one.
	Title string
	// Description is the summary of what this feature/plugin provides.
	Description string
	// Icon is the feature's card icon in the plugin manager: a Lucide icon key
	// (see PLUGIN_ICONS in web/src/shell/areas.tsx) or a logo image URL.
	//
	// Leave it empty when the plugin has a menu — the card falls back to that
	// menu's icon, so repeating it here is duplication that can drift. Set it
	// for a background service with no menu, or when the card should
	// deliberately differ from the sidebar.
	//
	// A string that isn't a known key is rendered literally, as text. That is
	// intentional (it lets a plugin ship an emoji) but it means a typo shows up
	// as copy rather than a broken image — web/src/shell/menuIcons.test.ts
	// guards the Go-declared menu icons against exactly that.
	Icon string
	// Category is the primary classification constant (CategoryMarketing, CategorySecurity, etc.).
	Category string
	// Tags are optional tags for extra sub-filtering (e.g. []string{"smtp", "webhook"}).
	Tags []string
	// Group joins sibling plugins under a single toggle. Plugins sharing a Group
	// are enabled/disabled together as one feature; empty means the plugin is its
	// own feature. The enablement key is Group when set, otherwise Name().
	Group string
	// Core marks always-on plumbing (e.g. license activation, buyer identity):
	// never gated, not shown in the plugin manager, cannot be disabled.
	Core bool
	// EnabledByDefault makes a (non-Core) feature active for a workspace that has
	// never toggled it — i.e. when no PluginSetting row exists. The feature stays
	// user-toggleable in the plugin manager; this only changes the pre-toggle
	// default from off (opt-in) to on (opt-out). Ignored when Core is set.
	EnabledByDefault bool
	// Requires lists names of plugins that must be mounted for this plugin to
	// function. Validated at boot time.
	Requires []string
}

// Describer is the optional interface a Plugin implements to supply Info.
type Describer interface {
	Describe() Info
}

// Describe returns a plugin's Info, or a zero-value default (all fields empty).
func Describe(p Plugin) Info {
	if d, ok := p.(Describer); ok {
		return d.Describe()
	}
	return Info{}
}

// FeatureKey is the enable/disable unit for a plugin: its group if set, else its
// name. Plugins in the same group share one key and toggle together.
func FeatureKey(p Plugin) string {
	if g := Describe(p).Group; g != "" {
		return g
	}
	return p.Name()
}

// FeatureIsCore reports whether the feature identified by key is always-on
// plumbing within the given plugin set.
//
// Core-ness belongs to the feature, not to a single member. Several plugins can
// share one feature key — a core half that serves the routes and a downstream half that
// only contributes content, say — and if either declares Core the feature as a
// whole is never gated. Deciding per plugin instead lets the halves disagree:
// the manager offers a toggle that the Core half ignores, so turning the feature
// "off" leaves its menu and routes live. Every enablement decision (route gate,
// menu filter, manager listing) goes through this.
func FeatureIsCore(plugins []Plugin, key string) bool {
	for _, p := range plugins {
		if FeatureKey(p) == key && Describe(p).Core {
			return true
		}
	}
	return false
}

package ontology

import (
	"fmt"

	core "github.com/octarq-org/octarq/server/internal/ontology"
)

// defaultIcon is the sidebar glyph for an entity that declares none. It must be
// a key in menuIcons, or the default would be the very thing this file refuses.
const defaultIcon = "database"

// menuIcons mirrors PLUGIN_ICONS in web/src/shell/areas.tsx, the single table
// that maps an icon key to a lucide component. The sidebar renders an unknown
// key as literal text, so a typo in a declaration shows a word where a glyph
// belongs; refusing it at load is the only check that reaches runtime values.
//
// A key added to PLUGIN_ICONS belongs here too. web/src/shell/menuIcons.test.ts
// parses Go menu literals but cannot see a declaration file, which is why this
// list exists at all.
var menuIcons = map[string]bool{
	"bell": true, "book": true, "book-open": true, "bot": true, "boxes": true,
	"credit-card": true, "database": true, "globe": true, "hard-drive": true,
	"key": true, "key-round": true, "layout-dashboard": true, "line-chart": true,
	"link-2": true, "mail": true, "message-square": true, "puzzle": true,
	"scroll-text": true, "send": true, "server": true, "settings": true,
	"shield": true, "shield-alert": true, "shopping-cart": true, "sparkles": true,
	"store": true, "ticket": true, "user": true, "users": true, "wallet": true,
	"webhook": true, "workflow": true,
}

// validateIcon refuses an icon the sidebar cannot render.
func validateIcon(spec core.Spec) error {
	if spec.Icon == "" {
		return nil
	}
	if !menuIcons[spec.Icon] {
		return fmt.Errorf("ontology: entity %q: icon %q is not a sidebar icon key; use one of the keys in web/src/shell/areas.tsx or leave it empty",
			spec.Slug, spec.Icon)
	}
	return nil
}

// iconFor is the declared icon, or the default.
func iconFor(spec core.Spec) string {
	if spec.Icon == "" {
		return defaultIcon
	}
	return spec.Icon
}

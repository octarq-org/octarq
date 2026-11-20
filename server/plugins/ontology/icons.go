package ontology

import (
	"fmt"

	core "github.com/octarq-org/octarq/server/ontology"
)

const defaultIcon = "database"

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

func iconFor(spec core.Spec) string {
	if spec.Icon == "" {
		return defaultIcon
	}
	return spec.Icon
}

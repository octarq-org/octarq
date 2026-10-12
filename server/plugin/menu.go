package plugin

// MenuItem represents a menu item exposed by a plugin for rendering in the UI.
type MenuItem struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`           // emoji or icon key
	Category string `json:"category"`       // default category
	Area     string `json:"area,omitempty"` // target area: "assets" | "operations" | "settings" or plugin-declared area
	Order    int    `json:"order,omitempty"`
	// RequiredRole hides the entry from anyone below this org role
	// ("member" / "admin" / "owner"); empty shows it to everyone. It is a UX
	// hint only — the endpoints behind the page do their own enforcement, and
	// must, since nothing stops a caller from requesting the path directly.
	//
	// Set it whenever every route behind the menu entry is role-gated. Omitting
	// it there produces a nav item that leads only to a permission error.
	RequiredRole string `json:"requiredRole,omitempty"`
}

// MenuProvider is an optional interface a Plugin may implement if it registers
// dynamic menu links for the frontend sidebar.
type MenuProvider interface {
	Menus() []MenuItem
}

// InstanceMenuProvider is the instance-console analog of MenuProvider: entries
// land in the /instance console rail, never in a tenant sidebar. A plugin
// implements it for deployment-wide surfaces (one config per deployment, not
// per workspace); the endpoint that serves these entries is instance-admin
// gated, and Menus()/InstanceMenus() are disjoint on purpose — the same page
// must never be reachable from both shells.
//
// Semantics differ from MenuProvider in three ways:
//   - Category is ignored: the console rail is flat, there are no groups.
//   - RequiredRole is ignored: instance admin is the only gate, and an org
//     role says nothing about instance privilege.
//   - Order is honoured, ascending; ties break on ID (lexicographic) so the
//     served list is stable.
type InstanceMenuProvider interface {
	InstanceMenus() []MenuItem
}

// Action is a create affordance a plugin exposes to the shell's global create
// menu and command palette. It is placement data, not behavior: Path is a route
// the shell navigates to, and the page behind it opens its own create flow.
type Action struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`     // route that opens the create flow
	Icon     string `json:"icon"`     // lucide key, same table as MenuItem.Icon
	Category string `json:"category"` // grouping label in the create menu
	Order    int    `json:"order,omitempty"`
	// RequiredRole mirrors MenuItem.RequiredRole exactly — same semantics,
	// same UX-hint-only caveat. The endpoint behind the create flow does its
	// own enforcement and must, since nothing stops a direct request.
	RequiredRole string `json:"requiredRole,omitempty"`
}

// ActionProvider is an optional interface a Plugin may implement to contribute
// entries to the shell's global create menu.
type ActionProvider interface {
	Actions() []Action
}

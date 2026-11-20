package plugin

type MenuItem struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Path         string `json:"path"`
	Icon         string `json:"icon"`
	Category     string `json:"category"`
	Area         string `json:"area,omitempty"`
	Order        int    `json:"order,omitempty"`
	RequiredRole string `json:"requiredRole,omitempty"`
	External     bool   `json:"external"`
	Feature      string `json:"-"`
}

type MenuProvider interface {
	Menus() []MenuItem
}

type InstanceMenuProvider interface {
	InstanceMenus() []MenuItem
}

type Action struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Path         string `json:"path"`
	Icon         string `json:"icon"`
	Category     string `json:"category"`
	Order        int    `json:"order,omitempty"`
	RequiredRole string `json:"requiredRole,omitempty"`
}

type ActionProvider interface {
	Actions() []Action
}

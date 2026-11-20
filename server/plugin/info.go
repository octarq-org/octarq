package plugin

const (
	CategoryMarketing      = "marketing"
	CategoryMessaging      = "messaging"
	CategorySecurity       = "security"
	CategoryInfrastructure = "infrastructure"
	CategoryCommerce       = "commerce"
	CategoryAI             = "ai"
	CategoryUtilities      = "utilities"
)

var ValidCategories = map[string]bool{
	CategoryMarketing:      true,
	CategoryMessaging:      true,
	CategorySecurity:       true,
	CategoryInfrastructure: true,
	CategoryCommerce:       true,
	CategoryAI:             true,
	CategoryUtilities:      true,
}

type Info struct {
	Title            string
	Description      string
	Icon             string
	Category         string
	Tags             []string
	Group            string
	Core             bool
	EnabledByDefault bool
	Requires         []string
}

type Describer interface {
	Describe() Info
}

func Describe(p Plugin) Info {
	if d, ok := p.(Describer); ok {
		return d.Describe()
	}
	return Info{}
}

func FeatureKey(p Plugin) string {
	if g := Describe(p).Group; g != "" {
		return g
	}
	return p.Name()
}

func FeatureIsCore(plugins []Plugin, key string) bool {
	for _, p := range plugins {
		if FeatureKey(p) == key && Describe(p).Core {
			return true
		}
	}
	return false
}

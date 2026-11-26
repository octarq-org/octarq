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

type CategoryInfo struct {
	Key    string            `json:"key"`
	Tone   string            `json:"tone"`
	Labels map[string]string `json:"labels"`
}

var pluginCategories = []CategoryInfo{
	{
		Key:  CategoryMarketing,
		Tone: "indigo",
		Labels: map[string]string{
			"en": "Marketing",
			"zh": "营销",
			"es": "Marketing",
			"pt": "Marketing",
			"ja": "マーケティング",
		},
	},
	{
		Key:  CategoryMessaging,
		Tone: "cyan",
		Labels: map[string]string{
			"en": "Messaging",
			"zh": "消息",
			"es": "Mensajería",
			"pt": "Mensagens",
			"ja": "メッセージング",
		},
	},
	{
		Key:  CategoryInfrastructure,
		Tone: "violet",
		Labels: map[string]string{
			"en": "Infrastructure",
			"zh": "基础设施",
			"es": "Infraestructura",
			"pt": "Infraestrutura",
			"ja": "インフラストラクチャ",
		},
	},
	{
		Key:  CategorySecurity,
		Tone: "amber",
		Labels: map[string]string{
			"en": "Security",
			"zh": "安全",
			"es": "Seguridad",
			"pt": "Segurança",
			"ja": "セキュリティ",
		},
	},
	{
		Key:  CategoryCommerce,
		Tone: "green",
		Labels: map[string]string{
			"en": "Commerce",
			"zh": "电商",
			"es": "Comercio",
			"pt": "Comércio",
			"ja": "コマース",
		},
	},
	{
		Key:  CategoryAI,
		Tone: "indigo",
		Labels: map[string]string{
			"en": "AI",
			"zh": "AI",
			"es": "IA",
			"pt": "IA",
			"ja": "AI",
		},
	},
	{
		Key:  CategoryUtilities,
		Tone: "neutral",
		Labels: map[string]string{
			"en": "Utilities",
			"zh": "工具",
			"es": "Utilidades",
			"pt": "Utilitários",
			"ja": "ユーティリティ",
		},
	},
}

func PluginCategories() []CategoryInfo {
	out := make([]CategoryInfo, len(pluginCategories))
	for i, c := range pluginCategories {
		labels := make(map[string]string, len(c.Labels))
		for lang, label := range c.Labels {
			labels[lang] = label
		}
		out[i] = CategoryInfo{Key: c.Key, Tone: c.Tone, Labels: labels}
	}
	return out
}

var ValidCategories = func() map[string]bool {
	out := make(map[string]bool, len(pluginCategories))
	for _, c := range pluginCategories {
		out[c.Key] = true
	}
	return out
}()

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

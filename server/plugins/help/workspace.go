package help

import (
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

var helpPlaceholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

var forbiddenHelpPlaceholder = regexp.MustCompile(`(?i)(secret|token|password|key|webhook|vault|credential)`)

var notConfiguredMarkers = map[string]string{
	"en": "(not configured)",
	"zh": "（未配置）",
	"es": "(sin configurar)",
	"pt": "(não configurado)",
	"ja": "（未設定）",
}

var helpSubstitutionKeys = validatedHelpSubstitutionKeys()

func validatedHelpSubstitutionKeys() []string {
	keys := plugin.HelpPlaceholders()
	for _, key := range keys {
		if err := validateHelpSubstitutionKey(key); err != nil {
			panic(err)
		}
	}
	return keys
}

func validateHelpSubstitutionKey(key string) error {
	if forbiddenHelpPlaceholder.MatchString(key) {
		return fmt.Errorf("help: substitution key %q exposes sensitive material", key)
	}
	if !plugin.IsHelpPlaceholder(key) {
		return fmt.Errorf("help: substitution key %q is not a declared placeholder", key)
	}
	return nil
}

func resolveHelpLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if idx := strings.IndexAny(lang, ",;"); idx >= 0 {
		lang = strings.TrimSpace(lang[:idx])
	}
	if idx := strings.Index(lang, "-"); idx >= 0 {
		lang = lang[:idx]
	}
	switch lang {
	case "en", "zh", "es", "pt", "ja":
		return lang
	default:
		return "en"
	}
}

func helpRequestLang(r *http.Request) string {
	if r == nil {
		return resolveHelpLang("")
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = r.Header.Get("Accept-Language")
	}
	return resolveHelpLang(lang)
}

func (p *Plugin) requestOrgID(r *http.Request) uint {
	if p == nil || p.host == nil || r == nil {
		return 0
	}
	session := p.host.Session()
	if session == nil {
		return 0
	}
	return session.OrgID(r)
}

func (p *Plugin) placeholderValues(orgID uint) map[string]string {
	values := make(map[string]string, len(helpSubstitutionKeys))
	var settings plugin.SettingsStore
	if p != nil && p.host != nil {
		settings = p.host.Settings()
	}
	if settings != nil {
		values["app_name"] = firstHelpSetting(
			settings.GetWorkspaceSetting(orgID, "app_name"),
			settings.GetGlobalSetting("title"),
			settings.GetGlobalSetting("app_name"),
		)
		values["domain"] = firstHelpSetting(
			settings.GetWorkspaceSetting(orgID, "base_domain"),
			settings.GetGlobalSetting("base_domain"),
		)
	}
	values["org_slug"] = p.orgSlug(orgID)
	return values
}

func firstHelpSetting(candidates ...string) string {
	for _, candidate := range candidates {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
	}
	return ""
}

func (p *Plugin) orgSlug(orgID uint) string {
	if p == nil || p.pctx == nil || p.pctx.DB == nil || orgID == 0 {
		return ""
	}
	var org models.Org
	if err := p.pctx.DB.Select("slug").First(&org, orgID).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(org.Slug)
}

func substituteHelpPlaceholders(src, lang string, values map[string]string) string {
	if !strings.Contains(src, "{{") {
		return src
	}
	marker := notConfiguredMarkers[resolveHelpLang(lang)]
	return helpPlaceholderPattern.ReplaceAllStringFunc(src, func(match string) string {
		parts := helpPlaceholderPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		key := parts[1]
		if !plugin.IsHelpPlaceholder(key) {
			return match
		}
		value := strings.TrimSpace(values[key])
		if value == "" {
			return marker
		}
		return escapeHelpPlaceholderValue(value)
	})
}

func escapeHelpPlaceholderValue(value string) string {
	escaped := html.EscapeString(value)
	var b strings.Builder
	b.Grow(len(escaped) + 4)
	for _, r := range escaped {
		switch r {
		case '\\', '*', '_', '[', ']', '`', '~', '|':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

package links

import (
	"fmt"

	"github.com/octarq-org/octarq/plugin"
)

// RegisterViews registers the core tenant views for short links and link events.
func RegisterViews(ctx *plugin.Context) {
	if ctx == nil || ctx.RegisterTenantView == nil {
		return
	}

	// tenant_links view
	ctx.RegisterTenantView(plugin.TenantView{
		Name: "tenant_links",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer", Description: "Short link unique identifier"},
			{Name: "owner_id", Type: "integer", Description: "Owning organization/workspace ID"},
			{Name: "host", Type: "text", Description: "Custom domain host (empty for default host)"},
			{Name: "slug", Type: "text", Description: "Short link URL slug path"},
			{Name: "target", Type: "text", Description: "Destination redirect URL"},
			{Name: "password", Type: "text", Description: "Link password protection (redacted)"},
			{Name: "note", Type: "text", Description: "Operator note or description"},
			{Name: "title", Type: "text", Description: "Link title"},
			{Name: "tags", Type: "text", Description: "Comma-separated tag list"},
			{Name: "expires_at", Type: "datetime", Description: "Link expiration timestamp"},
			{Name: "expired_url", Type: "text", Description: "Destination URL after link expiration"},
			{Name: "click_limit", Type: "integer", Description: "Maximum click threshold (0 for unlimited)"},
			{Name: "archived", Type: "boolean", Description: "Whether the link is archived"},
			{Name: "enabled", Type: "boolean", Description: "Whether the link redirect is currently active"},
			{Name: "routing_rules", Type: "text", Description: "JSON array of dynamic conditional routing rules"},
			{Name: "clicks", Type: "integer", Description: "Total click counter"},
			{Name: "created_at", Type: "datetime", Description: "Creation timestamp"},
			{Name: "updated_at", Type: "datetime", Description: "Last update timestamp"},
		},
		Sensitive: []string{"password"},
		Definition: func(orgID uint) string {
			return fmt.Sprintf("SELECT id, owner_id, host, slug, target, password, note, title, tags, expires_at, expired_url, click_limit, archived, enabled, routing_rules, clicks, created_at, updated_at FROM links WHERE owner_id = %d", orgID)
		},
	})

	// tenant_link_events view
	ctx.RegisterTenantView(plugin.TenantView{
		Name: "tenant_link_events",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer", Description: "Click event unique identifier"},
			{Name: "link_id", Type: "integer", Description: "Associated short link ID"},
			{Name: "created_at", Type: "datetime", Description: "Click event timestamp"},
			{Name: "ip", Type: "text", Description: "Visitor IP address"},
			{Name: "country", Type: "text", Description: "Two-letter country code (ISO 3166-1 alpha-2)"},
			{Name: "region", Type: "text", Description: "Geographic region / state"},
			{Name: "city", Type: "text", Description: "City name"},
			{Name: "device", Type: "text", Description: "Visitor device type"},
			{Name: "browser", Type: "text", Description: "Visitor browser family"},
			{Name: "os", Type: "text", Description: "Visitor operating system"},
			{Name: "referer", Type: "text", Description: "HTTP Referer header"},
			{Name: "ua", Type: "text", Description: "HTTP User-Agent header"},
			{Name: "fingerprint", Type: "text", Description: "Privacy-preserving device fingerprint hash"},
			{Name: "is_bot", Type: "boolean", Description: "Whether the visit was classified as a bot"},
			{Name: "variant", Type: "text", Description: "A/B testing split variant tag"},
			{Name: "utm_source", Type: "text", Description: "UTM campaign source parameter"},
			{Name: "utm_medium", Type: "text", Description: "UTM campaign medium parameter"},
			{Name: "utm_campaign", Type: "text", Description: "UTM campaign name parameter"},
		},
		Sensitive: []string{"fingerprint"},
		Definition: func(orgID uint) string {
			return fmt.Sprintf("SELECT le.id, le.link_id, le.created_at, le.ip, le.country, le.region, le.city, le.device, le.browser, le.os, le.referer, le.ua, le.fingerprint, le.is_bot, le.variant, le.utm_source, le.utm_medium, le.utm_campaign FROM link_events le INNER JOIN links l ON le.link_id = l.id WHERE l.owner_id = %d", orgID)
		},
	})
}

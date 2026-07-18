// Package dns is a built-in Core plugin: domain management and DNS — the
// operator's zones, per-zone DNS-provider credentials (Cloudflare, …), live
// record CRUD through the provider, and the DNS-verification posture (SPF/DMARC/
// DKIM for mail hosts, CNAME health for link hosts).
//
// It was lifted out of the monolithic internal/api.Handler (design in
// docs/CORE-PLUGIN-EXTRACTION.md) and now mounts through the same plugin
// contract Pro features use, marked Core so it is always on and never gated. It
// still shares the model types in internal/models (phase 1); the redirect engine
// and mail features consume the domain's host lists directly from the DB, and
// Pro plugins reach live DNS through the plugin.DNSManager this plugin provides
// as the "dns.manager" service (replacing the seam the Handler used to own).
package dns

import (
	"errors"
	"net"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// ServiceDNSManager is the registry name under which this plugin provides the
// plugin.DNSManager seam. The app wires ctx.DNS to resolve it, so Pro plugins
// (infra, ai MCP tools) reach live DNS without importing this package.
const ServiceDNSManager = "dns.manager"

var errNotFound = errors.New("not found")

// Plugin implements the octarq plugin contract for domain/DNS management.
type Plugin struct {
	db      *gorm.DB
	orgID   func(*http.Request) uint
	audit   func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	encrypt func(plaintext []byte) (string, error)
	decrypt func(encoded string) ([]byte, error)

	publishEvent func(orgID uint, event string, data any)

	// DNS resolvers, injectable so tests can stub them; default to net.
	lookupTXT   func(string) ([]string, error)
	lookupCNAME func(string) (string, error)
}

// Compile-time capability checks.
var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
)

// New constructs the dns plugin.
func New() *Plugin {
	return &Plugin{
		lookupTXT:   net.LookupTXT,
		lookupCNAME: net.LookupCNAME,
	}
}

func (p *Plugin) Name() string { return "dns" }

func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{Title: "Domains & DNS", Description: "Domain lifecycle and DNS management across Cloudflare and DNSPod.", EnabledByDefault: true}
}

// Models are the domain/DNS tables. In phase 1 they still live in
// internal/models and are migrated by the core; returning them here is harmless
// (idempotent AutoMigrate) and readies phase 2, where the types move into this
// package and this becomes their sole migration owner.
func (p *Plugin) Models() []any {
	return []any{&Domain{}, &ProviderAccount{}}
}

// Menus announces this plugin's sidebar entry so /api/menus only offers it
// when the plugin is mounted and enabled for the workspace.
func (p *Plugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "domains", Label: "Domains", Path: "/domains", Icon: "🌐", Category: "Assets", Order: 10},
	}
}

// Mount wires the plugin's dependencies from the shared context and registers
// its routes on the core API, then provides the DNS manager seam.
func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	if ctx.DB != nil {
		p.db = ctx.DB
	}
	if ctx.OrgID != nil {
		p.orgID = ctx.OrgID
	}
	if ctx.Audit != nil {
		p.audit = ctx.Audit
	}
	if ctx.Encrypt != nil {
		p.encrypt = ctx.Encrypt
	}
	if ctx.Decrypt != nil {
		p.decrypt = ctx.Decrypt
	}
	if ctx.PublishEvent != nil {
		p.publishEvent = ctx.PublishEvent
	}
	if ctx.RegisterWebhookEvent != nil {
		ctx.RegisterWebhookEvent(plugin.WebhookEventDef{Key: "domain.create", Group: "Domain", Title: "Domain Created", Description: "A domain was added to the workspace"})
		ctx.RegisterWebhookEvent(plugin.WebhookEventDef{Key: "domain.verify_failed", Group: "Domain", Title: "Domain Verification Failed", Description: "A domain's provider or DNS verification check failed"})
	}

	p.migrateLegacy()

	api := ctx.Huma
	if api != nil {
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/dns/providers", Summary: "DNS Providers", Tags: []string{"DNS"}}, p.dnsProviders)

		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/provider-accounts", Summary: "List Provider Accounts", Tags: []string{"Providers"}}, p.listProviderAccounts)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/provider-accounts", Summary: "Create Provider Account", Tags: []string{"Providers"}, DefaultStatus: 201}, p.createProviderAccount)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/provider-accounts/{id}", Summary: "Update Provider Account", Tags: []string{"Providers"}}, p.updateProviderAccount)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/provider-accounts/{id}", Summary: "Delete Provider Account", Tags: []string{"Providers"}}, p.deleteProviderAccount)

		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/domains/sync", Summary: "Sync Domains", Tags: []string{"Domains"}}, p.syncDomains)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/domains", Summary: "List Domains", Tags: []string{"Domains"}}, p.listDomains)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/domains", Summary: "Create Domain", Tags: []string{"Domains"}, DefaultStatus: 201}, p.createDomain)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/domains/{id}", Summary: "Update Domain", Tags: []string{"Domains"}}, p.updateDomain)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/domains/{id}", Summary: "Delete Domain", Tags: []string{"Domains"}}, p.deleteDomain)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/domains/{id}/verify-dns", Summary: "Verify Domain DNS", Tags: []string{"Domains"}}, p.verifyDomainDNS)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/domains/{id}/records", Summary: "List DNS Records", Tags: []string{"Domains"}}, p.listRecords)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/domains/{id}/records", Summary: "Create DNS Record", Tags: []string{"Domains"}, DefaultStatus: 201}, p.createRecord)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/domains/{id}/records/{rid}", Summary: "Update DNS Record", Tags: []string{"Domains"}}, p.updateRecord)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/domains/{id}/records/{rid}", Summary: "Delete DNS Record", Tags: []string{"Domains"}}, p.deleteRecord)
	}

	ctx.Provide(plugin.ServiceDNSManager, p.DNSManager())
	ctx.Provide("dns.overview", p.overview)
	ctx.Provide("dns.purge", p.purge)
	ctx.Provide("dns.export", p.exportData)
	ctx.Provide("domains.mcp_export", p.mcpExportDomains)
}

func (p *Plugin) purge(orgID uint) error {
	p.db.Where("owner_id = ?", orgID).Delete(&Domain{})
	p.db.Where("owner_id = ?", orgID).Delete(&ProviderAccount{})
	return nil
}

func (p *Plugin) exportData(orgID uint) map[string]any {
	var doms []Domain
	var provs []ProviderAccount
	p.db.Where("owner_id = ?", orgID).Find(&doms)
	p.db.Where("owner_id = ?", orgID).Find(&provs)
	return map[string]any{
		"domains":          doms,
		"providerAccounts": provs,
	}
}

func (p *Plugin) overview(orgID uint, includeBot bool) map[string]any {
	count := func(model any, conds ...any) int64 {
		var n int64
		q := p.db.Model(model).Where("owner_id = ?", orgID)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}
	return map[string]any{
		"domains":     count(&Domain{}),
		"linkDomains": count(&Domain{}, "for_link = ?", true),
		"mailDomains": count(&Domain{}, "for_mail = ?", true),
	}
}

// orgDB scopes a query to the caller's org.
func (p *Plugin) orgDB(r *http.Request) *gorm.DB {
	return p.db.Where("owner_id = ?", p.orgID(r))
}

// providerFor decrypts a domain's stored credentials and builds its DNS
// provider. Scoped to the domain's owning org as defense-in-depth.
func (p *Plugin) providerFor(dom Domain) (dnsprovider.Provider, error) {
	if dom.ProviderAccountID == 0 {
		return nil, errors.New("domain has no provider account configured")
	}
	var acc ProviderAccount
	if err := p.db.Where("id = ? AND owner_id = ?", dom.ProviderAccountID, dom.OrgID).
		First(&acc).Error; err != nil {
		return nil, errors.New("provider account not found")
	}
	if acc.Config == "" {
		return nil, errors.New("provider account has no credentials configured")
	}
	creds, err := p.decrypt(acc.Config)
	if err != nil {
		return nil, errors.New("stored API token could not be decrypted — re-save this provider's API token under Settings → DNS Providers (the encryption key or database changed since it was saved)")
	}
	return dnsprovider.New(acc.Type, creds)
}

func (p *Plugin) migrateLegacy() {
	if p.db == nil {
		return
	}
	if p.db.Migrator().HasColumn(&Domain{}, "provider") && p.db.Migrator().HasColumn(&Domain{}, "config") {
		var legacyDomains []struct {
			ID       uint
			Provider string
			Config   string
		}
		p.db.Raw("SELECT id, provider, config FROM domains WHERE provider_account_id = 0 OR provider_account_id IS NULL").Scan(&legacyDomains)
		for _, ld := range legacyDomains {
			if ld.Provider == "" {
				continue
			}
			var acc ProviderAccount
			if err := p.db.Where("config = ?", ld.Config).First(&acc).Error; err != nil {
				acc = ProviderAccount{
					OrgID:  1,
					Name:   ld.Provider + " (Migrated)",
					Type:   ld.Provider,
					Config: ld.Config,
				}
				p.db.Create(&acc)
			}
			p.db.Exec("UPDATE domains SET provider_account_id = ? WHERE id = ?", acc.ID, ld.ID)
		}
	}
}

// Package dns is a built-in Core plugin: domain management and DNS — the
// operator's zones, per-zone DNS-provider credentials (Cloudflare, …), live
// record CRUD through the provider, and the DNS-verification posture (SPF/DMARC/
// DKIM for mail hosts, CNAME health for link hosts).
//
// It was lifted out of the monolithic internal/api.Handler and now mounts
// through the same plugin contract Pro features use, marked Core so it is always on and never gated. It
// still shares the model types in internal/models (phase 1); the redirect engine
// and mail features consume the domain's host lists directly from the DB, and
// Pro plugins reach live DNS through the plugin.DNSManager this plugin provides
// as the "dns.manager" service (replacing the seam the Handler used to own).
package dns

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/dnsprovider"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// ServiceDNSManager is the registry name under which this plugin provides the
// plugin.DNSManager seam. The app wires ctx.DNS to resolve it, so Pro plugins
// (infra, ai MCP tools) reach live DNS without importing this package.
//
// It is an alias of plugin.ServiceDNSManager, not a second spelling of the same
// literal: the name is one coordinate shared by this provider and every
// consumer, so it gets one declaration. Two constants holding "dns.manager"
// compile fine after either is edited, and the wiring silently stops resolving.
const ServiceDNSManager = plugin.ServiceDNSManager

var (
	errNotFound = errors.New("not found")
	// errProviderAccountNotFound means the selected account exists in neither
	// registry: the caller's ID is stale or belongs to another workspace.
	errProviderAccountNotFound = errors.New("provider account not found")
	// errProviderCredentials means the account exists but its credentials could
	// not be loaded or decrypted. That is a server-side fault, never a 400.
	errProviderCredentials = errors.New("provider credentials unavailable")
)

// Plugin implements the octarq plugin contract for domain/DNS management.
type Plugin struct {
	db          *gorm.DB
	orgID       func(*http.Request) uint
	audit       func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	encrypt     func(plaintext []byte) (string, error)
	decrypt     func(encoded string) ([]byte, error)
	requireRole func(r *http.Request, min string) bool

	publishEvent func(orgID uint, event string, data any)
	ctx          *plugin.Context
	host         plugin.Host

	// DNS resolvers, injectable so tests can stub them; default to net.
	lookupTXT   func(string) ([]string, error)
	lookupCNAME func(string) (string, error)
}

// Host returns the host runtime interface.
func (p *Plugin) Host() plugin.Host { return p.host }

// SetHost sets the host runtime interface.
func (p *Plugin) SetHost(h plugin.Host) { p.host = h }

// Compile-time capability checks.
var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
	_ plugin.HelpDocsFS   = (*Plugin)(nil)
)

// Compile-time service contract assertions: these methods are provided to the
// registry under the named contract types in Mount. A signature drift here
// fails the build instead of silently breaking consumers' LookupServiceAs.
var (
	_ plugin.ExportFunc   = (*Plugin)(nil).exportData
	_ plugin.PurgeFunc    = (*Plugin)(nil).purge
	_ plugin.OverviewFunc = (*Plugin)(nil).overview
	_ plugin.MCPExporter  = (*Plugin)(nil).mcpExportDomains
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
	return plugin.Info{
		Title:            "Domains & DNS",
		Description:      "Domain lifecycle and DNS management across Cloudflare and DNSPod.",
		Category:         plugin.CategoryInfrastructure,
		Tags:             []string{"dns", "cloudflare", "dnspod"},
		EnabledByDefault: true,
	}
}

// Models are the domain/DNS tables. In phase 1 they still live in
// internal/models and are migrated by the core; returning them here is harmless
// (idempotent AutoMigrate) and readies phase 2, where the types move into this
// package and this becomes their sole migration owner.
func (p *Plugin) Models() []any {
	return []any{&Domain{}, &ProviderAccount{}, &DDNSToken{}}
}

// Menus announces this plugin's sidebar entry so /api/menus only offers it
// when the plugin is mounted and enabled for the workspace.
func (p *Plugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "domains", Label: "DNS", Path: "/domains", Icon: "globe", Category: "Network", Area: "assets", Order: 10},
	}
}

// Actions announces this plugin's global create affordances so /api/actions
// only offers them when the plugin is mounted and enabled for the workspace.
func (p *Plugin) Actions() []plugin.Action {
	return []plugin.Action{
		{ID: "create-domain", Label: "Add Domain", Path: "/domains?create=1", Icon: "globe", Category: "Network", Order: 10},
	}
}

// docs is this plugin's documentation directory and the only place its pages
// are declared. Adding one means adding "docs/<slug>.mdx" plus its ".zh.mdx"
// translation — nothing else. See plugin.HelpDocsFS.
//
//go:embed docs
var docs embed.FS

func (p *Plugin) HelpDocsFS() fs.FS { return docs }

// Mount wires the plugin's dependencies from the shared context and registers
// its routes on the core API, then provides the DNS manager seam.
func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	p.ctx = ctx
	if ctx.DB != nil {
		p.db = ctx.DB
	}
	p.host = plugin.EnsureHost(ctx)
	if p.host != nil {
		if p.host.Session() != nil {
			p.orgID = p.host.Session().OrgID
			p.requireRole = p.host.Session().RequireRole
		}
		if p.host.Crypto() != nil {
			p.encrypt = p.host.Crypto().Encrypt
			p.decrypt = p.host.Crypto().Decrypt
		}
		if p.host.Events() != nil {
			p.publishEvent = p.host.Events().PublishEvent
			p.host.Events().RegisterWebhookEvent(plugin.WebhookEventDef{Key: "domain.create", Group: "Domain", Title: "Domain Created", Description: "A domain was added to the workspace"})
			p.host.Events().RegisterWebhookEvent(plugin.WebhookEventDef{Key: "domain.verify_failed", Group: "Domain", Title: "Domain Verification Failed", Description: "A domain's provider or DNS verification check failed"})
		}
	}
	if ctx.Audit != nil {
		p.audit = ctx.Audit
	}

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
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/domains/{id}/email-blueprint", Summary: "Email DNS Blueprint", Tags: []string{"Domains"}}, p.emailBlueprint)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/domains/{id}/apply-email-blueprint", Summary: "Apply Email DNS Blueprint", Tags: []string{"Domains"}, DefaultStatus: 200}, p.applyEmailBlueprint)

		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/dns/ddns", Summary: "List DDNS Tokens", Tags: []string{"DDNS"}}, p.listDDNSTokens)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/dns/ddns", Summary: "Create DDNS Token", Tags: []string{"DDNS"}, DefaultStatus: 201}, p.createDDNSToken)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/dns/ddns/{id}", Summary: "Delete DDNS Token", Tags: []string{"DDNS"}}, p.deleteDDNSToken)

		huma.Register(api, huma.Operation{
			Method:   "GET",
			Path:     "/api/dns/ddns/update",
			Summary:  "DDNS Update (GET)",
			Tags:     []string{"DDNS"},
			Metadata: map[string]any{"public": true},
		}, p.updateDDNS)
		huma.Register(api, huma.Operation{
			Method:   "POST",
			Path:     "/api/dns/ddns/update",
			Summary:  "DDNS Update (POST)",
			Tags:     []string{"DDNS"},
			Metadata: map[string]any{"public": true},
		}, p.updateDDNS)
	}

	ctx.Provide(plugin.ServiceDNSManager, p.DNSManager())
	ctx.Provide(plugin.OverviewServiceName("dns"), plugin.OverviewFunc(p.overview))
	ctx.Provide(plugin.PurgeServiceName("dns"), plugin.PurgeFunc(p.purge))
	ctx.Provide(plugin.ExportServiceName("dns"), plugin.ExportFunc(p.exportData))
	ctx.Provide(plugin.MCPExportServiceName("domains"), plugin.MCPExporter(p.mcpExportDomains))
	ctx.Provide("dns.trust_proxy", SetTrustProxy)
}

func (p *Plugin) purge(orgID uint) error {
	// Collect the names before the rows go; see forgetOrigin. A purged
	// workspace's hostnames must stop resolving to it immediately, not when the
	// cache happens to expire.
	var names []string
	p.db.Model(&Domain{}).Where("owner_id = ?", orgID).Pluck("name", &names)
	p.db.Where("owner_id = ?", orgID).Delete(&Domain{})
	forgetOrigin(names...)
	p.db.Where("owner_id = ?", orgID).Delete(&ProviderAccount{})
	p.db.Where("owner_id = ?", orgID).Delete(&DDNSToken{})
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

// credentialHost returns the mounted Host, or a no-op host when the plugin has
// none (unit tests build the plugin directly). The no-op host reports
// ErrCredentialResolverUnavailable, which is exactly "no Integrations Hub here".
func (p *Plugin) credentialHost() plugin.Host {
	if p.host != nil {
		return p.host
	}
	return plugin.EnsureHost(nil)
}

// providerFor decrypts a domain's stored credentials and builds its DNS
// provider. Exactly one registry is authoritative: an instance with the
// Integrations Hub mounted resolves the account there and refuses a missing
// integration, while standalone Core reads the local provider_accounts table.
// Scoped to the domain's owning org.
func (p *Plugin) providerFor(dom Domain) (dnsprovider.Provider, error) {
	return p.providerForContext(context.Background(), dom)
}

func (p *Plugin) providerForContext(ctx context.Context, dom Domain) (dnsprovider.Provider, error) {
	if dom.ProviderAccountID == 0 {
		return nil, errors.New("domain has no provider account configured")
	}

	cred, err := p.credentialHost().LookupCredential(ctx, dom.OrgID, plugin.CredentialQuery{
		ID:           dom.ProviderAccountID,
		ProviderType: "dns",
	})
	if err == nil {
		if cred == nil || cred.OrgID != dom.OrgID {
			return nil, errProviderAccountNotFound
		}
		if len(cred.ConfigJSON) == 0 {
			return nil, fmt.Errorf("%w: integration %d has no credentials", errProviderCredentials, dom.ProviderAccountID)
		}
		return dnsprovider.New(cred.Provider, cred.ConfigJSON)
	}
	if errors.Is(err, plugin.ErrCredentialNotFound) {
		// The hub is mounted and owns DNS credentials: its answer is final.
		// Falling through to a same-numbered local row would bind the domain to
		// a different account, because the registries number rows independently.
		return nil, errProviderAccountNotFound
	}
	if !errors.Is(err, plugin.ErrCredentialResolverUnavailable) {
		return nil, fmt.Errorf("%w: %v", errProviderCredentials, err)
	}

	// Standalone Core: no Integrations Hub is mounted.
	var acc ProviderAccount
	if err := p.db.Where("id = ? AND owner_id = ?", dom.ProviderAccountID, dom.OrgID).
		First(&acc).Error; err != nil {
		return nil, errProviderAccountNotFound
	}
	if acc.Config == "" {
		return nil, fmt.Errorf("%w: provider account %d has no credentials", errProviderCredentials, acc.ID)
	}
	creds, err := p.decrypt(acc.Config)
	if err != nil {
		return nil, fmt.Errorf("%w: stored API token could not be decrypted. Re-save this provider's API token under Settings → DNS Providers (the encryption key or database changed since it was saved)", errProviderCredentials)
	}
	return dnsprovider.New(acc.Type, creds)
}

// hasRole reports whether the caller holds at least the given workspace role.
//
// A host that never wired RequireRole is refused rather than waved through. The
// gate protects destructive and credential-bearing operations, so "the host did
// not tell us who this is" has to mean no, not yes — an unwired seam would
// otherwise silently disable every role check in this plugin.
func (p *Plugin) hasRole(r *http.Request, min string) bool {
	if p.requireRole == nil {
		return false
	}
	return p.requireRole(r, min)
}

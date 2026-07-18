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

	// DNS resolvers, injectable so tests can stub them; default to net.
	lookupTXT   func(string) ([]string, error)
	lookupCNAME func(string) (string, error)
}

// Compile-time capability checks.
var (
	_ plugin.Plugin    = (*Plugin)(nil)
	_ plugin.Describer = (*Plugin)(nil)
)

// New constructs the dns plugin.
func New() *Plugin {
	return &Plugin{
		lookupTXT:   net.LookupTXT,
		lookupCNAME: net.LookupCNAME,
	}
}

func (p *Plugin) Name() string { return "dns" }

// Describe marks the plugin Core: always-on plumbing, never gated, not shown in
// the plugin manager — the same status the feature had as built-in core code.
func (p *Plugin) Describe() plugin.Info { return plugin.Info{Title: "Domains & DNS", Core: true} }

// Models are the domain/DNS tables. In phase 1 they still live in
// internal/models and are migrated by the core; returning them here is harmless
// (idempotent AutoMigrate) and readies phase 2, where the types move into this
// package and this becomes their sole migration owner.
func (p *Plugin) Models() []any {
	return []any{&Domain{}, &ProviderAccount{}}
}

// Mount wires the plugin's dependencies from the shared context and registers
// its routes on the core API, then provides the DNS manager seam.
func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	p.db = ctx.DB
	p.orgID = ctx.OrgID
	p.audit = ctx.Audit
	p.encrypt = ctx.Encrypt
	p.decrypt = ctx.Decrypt

	api := ctx.Huma
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

	ctx.Provide(ServiceDNSManager, p.DNSManager())
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

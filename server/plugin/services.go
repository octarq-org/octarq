package plugin

import (
	"context"
	"errors"
)

// Well-known service names for cross-plugin contracts, paired with the named
// contract types below. Providers register under these names in Mount with an
// explicit conversion to the matching named type; consumers resolve them with
// LookupAs / LookupServiceAs using the same named type. A provider signature
// drift therefore fails the build at the Provide call site instead of passing
// silently and dying at a runtime type assertion.

// ServiceMailSend is the well-known service name under which the transactional
// mail sender is provided (contract type MailSender).
const ServiceMailSend = "mail.send"

// ServiceMailReady is the well-known service name under which the mail plugin
// reports whether the instance can actually deliver transactional mail
// (contract type MailReady). It answers "is the system sender available",
// which is a different question from "is the mail plugin mounted": a mounted
// plugin with no sender cannot deliver a single message. The registration
// verification gate and the startup/API readiness reports consume this.
const ServiceMailReady = "mail.ready"

// ServiceMailSendSystem is the well-known service name under which the
// instance-level system mail sender is provided (contract type
// SystemMailSender). System mail — email verification, password reset,
// invites — must not depend on which org the recipient belongs to (at
// registration time there is no org yet), so it goes through the instance's
// system sender instead of the org-scoped mail.send contract.
const ServiceMailSendSystem = "mail.send.system"

// ServiceMailDispatcher is the well-known service name under which the inbound
// email handler registrar is provided (contract type EmailDispatcher).
const ServiceMailDispatcher = "mail.dispatcher"

// ServiceCloudUsage is the well-known service name under which the metered
// usage reporter is provided (contract type UsageMeter).
const ServiceCloudUsage = "cloud.usage"

// ServiceCloudTier is the well-known service name under which the hosted
// (Cloud) build provides the per-org subscription-tier resolver (contract type
// TierResolver). Plugins that must know an org's plan but cannot import the
// cloud module — the AI budget resolver reads aiCallsPerMonth for the tier —
// resolve it here. Nothing provides it on a self-hosted build.
const ServiceCloudTier = "cloud.tier"

// ServiceCloudQuota is the well-known service name under which the hosted
// (Cloud) build provides the plan/quota catalog backing per-org limits. Unlike
// the other names here the contract type is NOT declared in this package: the
// catalog interface (Limits/MeteredMetrics/PriceCents/…) is a downstream
// billing concept owned by the quota provider (e.g. pkg/quota.Source), and duplicating it
// here would create a second definition that can drift silently — Go asserts
// interfaces structurally, so a drifted copy fails the lookup at runtime with
// no compile error anywhere.
//
// The name lives here because it is the shared coordinate: the cloud module
// provides it and the ai and product modules consume it, and a string literal
// repeated across three modules is the thing that goes wrong when it changes.
// Providers MUST still convert at the Provide call site — with the provider-owned
// type: ctx.Provide(plugin.ServiceCloudQuota, quota.Source(p.quotaSrc)) — and
// consumers MUST resolve it with LookupAs[quota.Source].
const ServiceCloudQuota = "cloud.quota"

// ServiceLinkResolve is the well-known service name under which the links
// plugin provides slug resolution for the public abuse-report form (contract
// type LinkResolver).
const ServiceLinkResolve = "links.resolve"

// ServiceMailEmailGet is the well-known service name under which the mail
// plugin provides email lookup for the AI summarizer (contract type
// EmailGetter).
const ServiceMailEmailGet = "mail.email.get"

// ServiceStorage is the well-known service name under which the storage
// service SPI is provided (contract type StorageService).
const ServiceStorage = "storage.service"

// ServiceNotificationRouter is the well-known service name under which the
// notification router SPI is provided.
const ServiceNotificationRouter = "notification.router"

// ServiceCron is the well-known service name under which the cron engine
// is provided (contract type CronService).
const ServiceCron = "cron.service"

// ServiceHealthProvider is the well-known service name under which a health
// provider is registered (contract type HealthProvider).
const ServiceHealthProvider = "health.provider"

// ServiceHealthProvidersAll is the service name under which all registered
// health providers are resolved as []HealthProvider.
const ServiceHealthProvidersAll = "health.providers.all"

// HealthProviderServiceName returns the well-known service name under which
// a specific named health provider is registered (e.g. "health.provider.runtime").
func HealthProviderServiceName(name string) string {
	return "health.provider." + name
}

// CleanupServiceName returns the well-known "<pluginName>.cleanup" service name
// under which a plugin provides its retention cleanup (contract type
// CleanupFunc). The app looks every registered plugin's cleanup service up
// under this name before launching the retention sweeper.
func CleanupServiceName(pluginName string) string {
	return pluginName + ".cleanup"
}

// ExportServiceName returns the well-known "<pluginName>.export" service name
// under which a plugin provides its contribution to the org account export
// (contract type ExportFunc). The app looks every registered plugin's export
// service up under this name while assembling GET /api/account/export.
func ExportServiceName(pluginName string) string {
	return pluginName + ".export"
}

// PurgeServiceName returns the well-known "<pluginName>.purge" service name
// under which a plugin provides its tenant-data erasure (contract type
// PurgeFunc). The app looks every registered plugin's purge service up under
// this name before deleting the org (DELETE /api/account/data).
func PurgeServiceName(pluginName string) string {
	return pluginName + ".purge"
}

// MCPExportServiceName returns the well-known "<resource>.mcp_export" service
// name under which a plugin provides that resource's MCP export (contract type
// MCPExporter). The key is the RESOURCE, not the plugin: mail provides both
// "emails" and "mailboxes", dns provides "domains".
func MCPExportServiceName(resource string) string {
	return resource + ".mcp_export"
}

// OverviewServiceName returns the well-known "<pluginName>.overview" service
// name under which a plugin provides its overview statistics (contract type
// OverviewFunc). The app looks every registered plugin's overview service up
// under this name while assembling the overview endpoint.
func OverviewServiceName(pluginName string) string {
	return pluginName + ".overview"
}

// MailSender is the cross-plugin contract for sending transactional email
// through the org's configured SMTP sender. Changing this signature breaks the
// contract: the provider (the mail plugin) and every consumer (app, core API)
// must change together, which the explicit conversion at the Provide call site
// enforces at compile time.
type MailSender func(orgID uint, to, subject, htmlBody, textBody string) error

// SystemMailSender is the cross-plugin contract for sending instance-level
// system mail (email verification, password reset, invites) through the
// instance's system sender. It carries no orgID: those flows run before any
// membership exists (registration) or for recipients whose org is irrelevant,
// and the instance setting (or the deterministic lowest-id fallback) picks the
// sender. Changing this signature breaks the contract: provider and consumers
// must change together, which the explicit conversion at the Provide call site
// enforces at compile time.
type SystemMailSender func(to, subject, htmlBody, textBody string) error

// MailReady is the cross-plugin contract for the "can this instance send mail"
// question: true when at least one SMTP sender is configured, false otherwise.
// The provider (the mail plugin) answers from its own database; consumers
// (core API, app readiness) must treat "service absent" as "not ready" — a
// mounted plugin with no sender configured is exactly the instance that cannot
// deliver a single message.
type MailReady func() bool

// EmailDispatcher is the cross-plugin contract for registering inbound-email
// handlers. The provider (the mail plugin) registers handlers to run after each
// inbound email is stored; consumers that are too early (Mount-time, before the
// mail plugin mounted) may defer handlers and flush them once this service is
// available. Changing this signature breaks the contract: provider and
// consumers must change together.
type EmailDispatcher func(handler func(EmailEvent))

// UsageMeter is the cross-plugin contract for reporting metered tenant
// consumption (a link redirect, an email send) to the billing backend. The
// provider lives in a downstream cloud module and
// registers it under ServiceCloudUsage; this OSS side defines the contract and
// consumes it. On self-hosted builds nothing provides the service and call
// sites simply find nothing and proceed. Changing this signature breaks the
// contract: the provider and every consumer must change together.
type UsageMeter func(orgID uint, metric string, n int64)

// TierResolver is the cross-plugin contract for resolving an org's
// subscription tier ("", "solo", "team", …; "" means Free — an org with no
// subscription row is a Free org, not an error). The provider lives in a
// downstream cloud module and registers it under
// ServiceCloudTier; this OSS side owns the contract so both halves name the
// same type. On self-hosted builds nothing provides the service and consumers
// simply find nothing and fall back to their own defaults.
//
// A signature drift fails silently and expensively: the lookup misses, the AI
// budget falls back to the embedded catalog, and a paying org is metered at
// free-plan limits with nothing in the logs. Providers MUST convert at the
// Provide call site — ctx.Provide(plugin.ServiceCloudTier,
// plugin.TierResolver(p.planForOrg)) — and consumers MUST resolve it with
// LookupServiceAs[plugin.TierResolver] / LookupAs[plugin.TierResolver], so the
// drift is a compile error where the provider is written rather than a runtime
// assertion failure nobody sees.
type TierResolver func(orgID uint) string

// CleanupFunc is the cross-plugin contract for a plugin's retention cleanup,
// run by the app's sweeper with the configured data-retention window. The
// provider registers it under CleanupServiceName(pluginName). Changing this
// signature breaks the contract: provider and the app must change together.
type CleanupFunc func(ctx context.Context, retentionDays int)

// ExportFunc is the cross-plugin contract for a plugin's contribution to the
// org account export (GET /api/account/export): the plugin returns a
// flat map of key/value pairs that the app merges into the export body, one
// entry per key. Any plugin holding tenant data worth exporting provides it
// under ExportServiceName(pluginName) — the OSS plugins here, and several
// downstream modules. Providing it is optional; a plugin that does
// not is simply skipped.
//
// A signature drift fails silently in production: the plugin's data is simply
// missing from the user's export. Changing this signature breaks the contract:
// every provider and the app must change together, which the explicit
// conversion at the Provide call site enforces at compile time.
type ExportFunc func(orgID uint) map[string]any

// PurgeFunc is the cross-plugin contract for a plugin's tenant-data erasure,
// run by the app before the org row itself is deleted (DELETE
// /api/account/data). Every plugin that stores tenant rows provides it under
// PurgeServiceName(pluginName) — the OSS plugins here, and downstream
// modules.
//
// A signature drift fails silently in production: the plugin's tenant data
// survives the deletion and the operator believes the workspace was erased — a
// data-retention/compliance problem. Changing this signature breaks the
// contract: every provider and the app must change together, which the
// explicit conversion at the Provide call site enforces at compile time.
type PurgeFunc func(orgID uint) error

// OverviewFunc is the cross-plugin contract for a plugin's contribution to
// the overview endpoint: the plugin returns a flat map of statistics the app
// merges into the overview body, warning on a key collision. It is provided
// under OverviewServiceName(pluginName) by the OSS plugins (links, mail, dns);
// no downstream module currently provides one. Changing this signature breaks the
// contract: every provider and the app must change together.
type OverviewFunc func(orgID uint, includeBot bool) map[string]any

// LinkResolver is the cross-plugin contract for attributing a reported slug
// to the link actually served at (host, slug), backing the public abuse-report
// form. The provider (the links plugin) registers it under ServiceLinkResolve.
// Changing this signature breaks the contract: provider and the app must
// change together.
type LinkResolver func(host, slug string) (target string, orgID uint, ok bool)

// EmailGetter is the cross-plugin contract for fetching one inbound email for
// the AI summarizer. The provider (the mail plugin) registers it under
// ServiceMailEmailGet. Changing this signature breaks the contract: provider
// and the app must change together.
type EmailGetter func(orgID uint, id uint) (from, subject, body string, ok bool)

// MCPExporter is the cross-plugin contract for exporting one resource through
// the MCP export tool. It is keyed by RESOURCE, not by plugin name — the mail
// plugin provides two ("emails", "mailboxes") — so providers register it under
// MCPExportServiceName(resource).
//
// A signature drift fails silently and confusingly: the lookup succeeds, the
// type assertion does not, and the tool answers "unknown resource" for a
// resource that is in fact mounted. Changing this signature breaks the
// contract: every provider and the MCP server must change together, which the
// explicit conversion at the Provide call site enforces at compile time.
type MCPExporter func(ctx context.Context, orgID uint) (any, error)

// ServiceDNSManager is the well-known service name under which the DNS manager
// is provided (contract type DNSManager). This is the single declaration of the
// name: the dns plugin re-exports it rather than declaring a second constant
// with the same literal, so editing it here moves both halves of the wiring.
//
// Providers MUST convert at the Provide call site —
// ctx.Provide(plugin.ServiceDNSManager, plugin.DNSManager(mgr)) — and consumers
// MUST resolve it with LookupAs[plugin.DNSManager] rather than a hand-rolled
// type assertion, so there is one assertion implementation.
const ServiceDNSManager = "dns.manager"

// ServiceMailStorageProvider is the well-known service name under which the raw
// email storage provider is registered (contract type StorageProvider).
//
// Providers MUST convert at the Provide call site —
// ctx.Provide(plugin.ServiceMailStorageProvider, plugin.StorageProvider(prov))
// — and consumers MUST resolve it with LookupAs[plugin.StorageProvider].
// Providing the concrete type unconverted compiles even after the concrete type
// stops satisfying StorageProvider; the failure then surfaces as raw email
// silently falling back to database storage at runtime.
const ServiceMailStorageProvider = "mail.storage_provider"

// ErrStorageNotFound is returned when a storage object does not exist.
var ErrStorageNotFound = errors.New("storage object not found")

// StorageProvider defines the blob storage contract for raw email content.
type StorageProvider interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Stat(ctx context.Context, key string) (int64, error)
}

// ServiceQuotaChecker is the well-known service name under which a per-org
// quota checker is registered (contract type QuotaChecker). Only the hosted
// (Cloud) build provides one; a self-hosted install has no quota concept at all
// and every call site simply finds nothing here and proceeds.
//
// Providers MUST convert at the Provide call site —
// ctx.Provide(plugin.ServiceQuotaChecker, plugin.QuotaChecker(p)) — and
// consumers MUST resolve it with LookupAs[plugin.QuotaChecker] (CheckQuota does
// exactly that). Providing the concrete plugin value unconverted compiles fine
// the day the concrete type stops satisfying QuotaChecker, and the only symptom
// is that every quota stops being enforced: "service absent" and "service
// present but wrong type" are indistinguishable to the consumer, and both read
// as "self-hosted, no quotas".
const ServiceQuotaChecker = "quota.checker"

// QuotaChecker decides whether an org may consume n more of a metered
// resource. Metric names are the keys of the pricing catalog's per-plan quota
// map ("links", "customDomains", "mailOutPerMonth", ...).
//
// The three outcomes are deliberately distinct and must not be collapsed:
//
//   - nil                  → allowed. Either under quota, or over it on a paid
//     plan where the excess is billed as metered
//     overage rather than refused.
//   - ErrQuotaExceeded     → the org has a quota for this metric and has used
//     it up. Callers answer 429.
//   - ErrQuotaUnavailable  → the plan does not include this capability at all
//     (a Free org sending outbound mail). Callers answer
//     402: this is an upgrade prompt, not a rate limit.
//
// Implementations must be safe for concurrent use.
type QuotaChecker interface {
	Check(ctx context.Context, orgID uint, metric string, n int64) error
}

var (
	// ErrQuotaExceeded means the org used up an allowance it does have.
	ErrQuotaExceeded = errors.New("quota exceeded")
	// ErrQuotaUnavailable means the org's plan does not include this capability.
	ErrQuotaUnavailable = errors.New("capability not included in plan")
)

// DNSRecord is a provider-agnostic DNS record, mirroring the fields of octarq's
// internal dnsprovider.Record using only stable types so plugins in a separate
// module never import internal packages. An empty ID on a write means "create".
type DNSRecord struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // A, AAAA, CNAME, TXT, MX, …
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	Comment  string `json:"comment"`
	Priority *int   `json:"priority,omitempty"`
}

// DNSManager is the DNS-management seam exposed to plugins via Context.DNS. All
// operations take a octarq domain ID and resolve its zone + provider internally.
//
// Every operation also takes the org whose behalf it acts on, and resolves the
// domain scoped to that org: a domain id alone is a bare primary key, and these
// calls write into a real zone with that domain's stored provider credentials.
// Passing the caller's own org is what keeps one tenant out of another's DNS
// even when the calling plugin forgets to check ownership itself. orgID is
// required — 0 is rejected rather than treated as "any org".
type DNSManager interface {
	// List returns all records in the domain's zone.
	List(ctx context.Context, orgID, domainID uint) ([]DNSRecord, error)
	// Set creates the record when r.ID is empty, otherwise updates it. Returns
	// the stored record.
	Set(ctx context.Context, orgID, domainID uint, r DNSRecord) (DNSRecord, error)
	// Delete removes a record by provider record ID.
	Delete(ctx context.Context, orgID, domainID uint, recordID string) error
}

// LinkCreator is the short-link creation seam the links plugin publishes under
// the service name "links.create" (resolve it lazily with
// plugin.LookupAs[plugin.LinkCreator]). It lets any plugin turn a URL into a
// short link — the programmatic equivalent of POST /api/links — without
// importing the links plugin. It degrades gracefully: a consumer must tolerate
// the service being absent (links not composed into this build).
type LinkCreator interface {
	// CreateLink creates an enabled short link for targetURL in orgID's
	// workspace on the default host with a random slug, and returns the slug.
	// targetURL must be an http(s) URL.
	CreateLink(ctx context.Context, orgID uint, targetURL string) (slug string, err error)
}

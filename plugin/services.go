package plugin

import "context"

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
// catalog interface (Limits/MeteredMetrics/PriceCents/…) is a commercial
// billing concept owned by octarq-pro's pkg/quota.Source, and duplicating it
// here would create a second definition that can drift silently — Go asserts
// interfaces structurally, so a drifted copy fails the lookup at runtime with
// no compile error anywhere.
//
// The name lives here because it is the shared coordinate: the cloud module
// provides it and the ai and product modules consume it, and a string literal
// repeated across three modules is the thing that goes wrong when it changes.
// Providers MUST still convert at the Provide call site — with the Pro-owned
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
// provider lives in the Pro cloud module (octarq-pro, not this repository) and
// registers it under ServiceCloudUsage; this OSS side defines the contract and
// consumes it. On self-hosted builds nothing provides the service and call
// sites simply find nothing and proceed. Changing this signature breaks the
// contract: the Pro provider and every consumer must change together.
type UsageMeter func(orgID uint, metric string, n int64)

// TierResolver is the cross-plugin contract for resolving an org's
// subscription tier ("", "solo", "team", …; "" means Free — an org with no
// subscription row is a Free org, not an error). The provider lives in the Pro
// cloud module (octarq-pro, not this repository) and registers it under
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
// under ExportServiceName(pluginName) — the OSS plugins here, and several of
// the Pro modules in octarq-pro. Providing it is optional; a plugin that does
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
// PurgeServiceName(pluginName) — the OSS plugins here, and every Pro module in
// octarq-pro.
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
// no Pro module currently provides one. Changing this signature breaks the
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

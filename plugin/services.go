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
// (contract type MailReady). It answers "is at least one SMTP sender
// configured", which is a different question from "is the mail plugin mounted":
// a mounted plugin with no sender cannot deliver a single message.
const ServiceMailReady = "mail.ready"

// ServiceMailDispatcher is the well-known service name under which the inbound
// email handler registrar is provided (contract type EmailDispatcher).
const ServiceMailDispatcher = "mail.dispatcher"

// ServiceCloudUsage is the well-known service name under which the metered
// usage reporter is provided (contract type UsageMeter).
const ServiceCloudUsage = "cloud.usage"

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

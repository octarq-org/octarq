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

// ServiceMailDispatcher is the well-known service name under which the inbound
// email handler registrar is provided (contract type EmailDispatcher).
const ServiceMailDispatcher = "mail.dispatcher"

// ServiceCloudUsage is the well-known service name under which the metered
// usage reporter is provided (contract type UsageMeter).
const ServiceCloudUsage = "cloud.usage"

// CleanupServiceName returns the well-known "<pluginName>.cleanup" service name
// under which a plugin provides its retention cleanup (contract type
// CleanupFunc). The app looks every registered plugin's cleanup service up
// under this name before launching the retention sweeper.
func CleanupServiceName(pluginName string) string {
	return pluginName + ".cleanup"
}

// MailSender is the cross-plugin contract for sending transactional email
// through the org's configured SMTP sender. Changing this signature breaks the
// contract: the provider (the mail plugin) and every consumer (app, core API)
// must change together, which the explicit conversion at the Provide call site
// enforces at compile time.
type MailSender func(orgID uint, to, subject, htmlBody, textBody string) error

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

package plugin

import "time"

// EmailEvent is a stable, external snapshot of a freshly received inbound email,
// delivered to handlers registered via Context.OnEmail. It mirrors only the
// fields a plugin needs so plugins never import octarq's internal/models. The full
// row (including Raw RFC822 bytes, e.g. for attachment OCR) remains reachable
// via the shared DB using ID.
//
// This is the low-latency entry point Inbox AI needs (summary/classification,
// OTP extraction): the core fires it the moment an email is stored, instead of
// a plugin having to poll for unsummarized rows.
type EmailEvent struct {
	ID         uint      // emails.id — load the full row from the DB if more is needed
	MailboxID  uint      // owning mailbox
	OrgID      uint      // tenant scope (mailbox owner) for org-scoped processing
	From       string    // envelope/header From
	To         string    // recipient the mail was routed to
	Subject    string    // header Subject
	Text       string    // plaintext body (may be empty if HTML-only)
	HTML       string    // HTML body (may be empty)
	ReceivedAt time.Time // when the MTA received it
}

// WebhookEventDef describes a webhook event a plugin can fire via
// Context.PublishEvent, so the dashboard's webhook editor can offer it for
// subscription. It mirrors the core registry's definition using only stable
// types so plugins in a separate module never import octarq's internal packages.
type WebhookEventDef struct {
	Key         string // e.g. "link.click"
	Group       string // display group, e.g. "Links"
	Title       string // e.g. "Link Clicked"
	Description string // when the event fires
}

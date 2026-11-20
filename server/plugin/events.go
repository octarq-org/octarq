package plugin

import "time"

type EmailEvent struct {
	ID         uint
	MailboxID  uint
	OrgID      uint
	From       string
	To         string
	Subject    string
	Text       string
	HTML       string
	ReceivedAt time.Time
}

type WebhookEventDef struct {
	Key         string
	Group       string
	Title       string
	Description string
}

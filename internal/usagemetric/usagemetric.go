// Package usagemetric names the metered metrics stored in the usage_aggregates
// table. Every RecordUsage/recordUsage call in this repo must pass one of
// these constants, never a hand-written literal.
//
// The other half of the equation is the quota side, which decides which
// metrics are actually enforced. It lives in downstream quota packages
// (metricNames) and maps quota keys to the same strings:
//
//	clicksPerMonth  -> "clicks"
//	mailOutPerMonth -> "mailOut"
//	aiCallsPerMonth -> "aiCalls"
//	mailInPerMonth  -> "mailIn"
//
// A metric name the quota side never reads is an allowance nobody enforces.
// usagemetric_test.go pins the constants below to that canonical set, so the
// two lists cannot silently drift apart again.
package usagemetric

const (
	// Clicks is a tracked short-link click (quota key "clicksPerMonth").
	Clicks = "clicks"

	// MailOut is one outbound message per recipient (quota key
	// "mailOutPerMonth"). Both outbound paths — the send API and
	// transactional sends — meter under this name.
	MailOut = "mailOut"

	// MailIn is one inbound message (quota key "mailInPerMonth").
	MailIn = "mailIn"

	// RawBytes is the number of bytes of inbound mail stored. The quota side
	// has no consumer for it today (it is absent from pkg/quota's
	// metricNames); it is reserved for future storage billing.
	RawBytes = "mail.raw_bytes"
)

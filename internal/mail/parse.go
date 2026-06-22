// Package mail parses inbound MIME messages and sends outbound mail via SMTP.
package mail

import (
	"io"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
)

func init() {
	// Allow non-UTF-8 charsets in headers/bodies.
	message.CharsetReader = charset.Reader
}

// Parsed is the normalized result of reading a raw RFC822 message.
type Parsed struct {
	MessageID  string
	From       string
	To         string
	Subject    string
	Text       string
	HTML       string
	ReceivedAt time.Time
	Raw        []byte
}

// Parse reads a raw email and extracts the fields led stores.
func Parse(raw []byte) (*Parsed, error) {
	mr, err := mail.CreateReader(strings.NewReader(string(raw)))
	if err != nil {
		// Fall back to a minimal record so delivery is never silently dropped.
		return &Parsed{Raw: raw, ReceivedAt: time.Now(), Subject: "(unparseable message)"}, nil
	}
	h := mr.Header
	p := &Parsed{Raw: raw, ReceivedAt: time.Now()}
	p.MessageID, _ = h.MessageID()
	p.Subject, _ = h.Subject()
	if addrs, err := h.AddressList("From"); err == nil && len(addrs) > 0 {
		p.From = addrs[0].Address
	}
	if addrs, err := h.AddressList("To"); err == nil && len(addrs) > 0 {
		p.To = addrs[0].Address
	}
	if t, err := h.Date(); err == nil && !t.IsZero() {
		p.ReceivedAt = t
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if ih, ok := part.Header.(*mail.InlineHeader); ok {
			ct, _, _ := ih.ContentType()
			b, _ := io.ReadAll(part.Body)
			if strings.HasPrefix(ct, "text/html") {
				p.HTML = string(b)
			} else if strings.HasPrefix(ct, "text/plain") {
				p.Text = string(b)
			}
		}
	}
	return p, nil
}

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

// Attachment is metadata for a non-inline message part.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int    `json:"size"`
}

// AuthResults holds the outcome of SPF, DKIM, and DMARC checks as reported
// in the Authentication-Results header added by the receiving MTA.
type AuthResults struct {
	SPF   string `json:"spf"`   // pass|fail|softfail|neutral|none|temperror|permerror
	DKIM  string `json:"dkim"`  // pass|fail|none|temperror|permerror
	DMARC string `json:"dmarc"` // pass|fail|none|temperror|permerror
}

// Parsed is the normalized result of reading a raw RFC822 message.
type Parsed struct {
	MessageID   string
	From        string
	To          string
	Subject     string
	Text        string
	HTML        string
	Attachments []Attachment
	ReceivedAt  time.Time
	Raw         []byte
	Auth        AuthResults
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
	// Authentication-Results may appear on multiple lines; merge all values.
	for _, v := range h.Header.Values("Authentication-Results") {
		parseAuthResults(v, &p.Auth)
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch hdr := part.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := hdr.ContentType()
			b, _ := io.ReadAll(part.Body)
			if strings.HasPrefix(ct, "text/html") {
				p.HTML = string(b)
			} else if strings.HasPrefix(ct, "text/plain") {
				p.Text = string(b)
			}
		case *mail.AttachmentHeader:
			ct, _, _ := hdr.ContentType()
			filename, _ := hdr.Filename()
			b, _ := io.ReadAll(part.Body)
			p.Attachments = append(p.Attachments, Attachment{
				Filename: filename, ContentType: ct, Size: len(b),
			})
		}
	}
	return p, nil
}

// parseAuthResults extracts spf/dkim/dmarc result tokens from one
// Authentication-Results header value.
//
// Header format (RFC 8601):
//
//	Authentication-Results: mx.example.com;
//	  spf=pass smtp.mailfrom=example.com;
//	  dkim=pass header.i=@example.com;
//	  dmarc=pass (p=NONE) header.from=example.com
func parseAuthResults(hdr string, out *AuthResults) {
	// Lowercase once; result tokens are case-insensitive.
	s := strings.ToLower(hdr)
	for _, method := range []struct {
		name   string
		target *string
	}{
		{"spf=", &out.SPF},
		{"dkim=", &out.DKIM},
		{"dmarc=", &out.DMARC},
	} {
		idx := 0
		for {
			pos := strings.Index(s[idx:], method.name)
			if pos < 0 {
				break
			}
			matchPos := idx + pos
			idx = matchPos + len(method.name)

			// Ensure match is at the start of the header string, or preceded by
			// a token separator (whitespace, semicolon, comma). This avoids false
			// matches like "x-not-dkim=pass".
			if matchPos > 0 {
				prev := s[matchPos-1]
				if prev != ' ' && prev != '\t' && prev != ';' && prev != ',' {
					continue
				}
			}

			rest := s[matchPos+len(method.name):]
			// Result token ends at the next whitespace, semicolon, or end.
			end := strings.IndexAny(rest, " \t\r\n;(")
			if end < 0 {
				end = len(rest)
			}
			token := strings.TrimSpace(rest[:end])
			if token != "" && *method.target == "" {
				*method.target = token
			}
			break
		}
	}
}

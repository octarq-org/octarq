// Package mail parses inbound MIME messages and sends outbound mail via SMTP.
package mail

import (
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
)

// Resource limits for parsing untrusted inbound mail. A single malformed or
// hostile message must not be able to exhaust memory.
const (
	maxPartBytes = 10 << 20 // 10 MiB read from any single MIME part
	maxParts     = 200      // maximum number of MIME parts walked
)

// readLimited reads up to maxPartBytes from r, discarding the rest so the
// underlying reader is drained without buffering an unbounded payload.
// It returns the read bytes, whether content was truncated, and the total size.
func readLimited(r io.Reader) (b []byte, truncated bool, totalBytes int) {
	b, _ = io.ReadAll(io.LimitReader(r, maxPartBytes))
	nDiscarded, _ := io.Copy(io.Discard, r)
	totalBytes = len(b) + int(nDiscarded)
	truncated = nDiscarded > 0
	return b, truncated, totalBytes
}

func init() {
	// Allow non-UTF-8 charsets in headers/bodies.
	message.CharsetReader = charset.Reader
}

// Attachment is metadata for a message part.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int    `json:"size"`
	Inline      bool   `json:"inline,omitempty"`
	ContentID   string `json:"contentId,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
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
	MessageID       string
	From            string
	To              string
	Subject         string
	Text            string
	HTML            string
	Attachments     []Attachment
	ReceivedAt      time.Time
	Raw             []byte
	Auth            AuthResults
	PartErrors      int
	MaxPartsReached bool
}

// Parse reads a raw email and extracts the fields octarq stores.
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
	for _, v := range h.Values("Authentication-Results") {
		parseAuthResults(v, &p.Auth)
	}

	parts := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			p.PartErrors++
			slog.Warn("failed to parse mime part", "message_id", p.MessageID, "error", err)
			continue
		}
		if parts++; parts > maxParts {
			p.PartErrors++
			p.MaxPartsReached = true
			slog.Warn("max mime parts limit reached", "message_id", p.MessageID, "limit", maxParts)
			break
		}
		switch hdr := part.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := hdr.ContentType()
			b, truncated, totalBytes := readLimited(part.Body)
			if strings.HasPrefix(ct, "text/html") {
				if p.HTML == "" && len(b) > 0 {
					p.HTML = string(b)
				} else if len(b) > 0 {
					// According to RFC 2046 (multipart/alternative), later parts are "richer"
					// representations, but earlier parts are more primary/authentic.
					// We keep the first non-empty text/html part to prevent malicious or
					// unexpected later parts from silently overwriting the primary body text,
					// and increment PartErrors to flag the duplicate.
					p.PartErrors++
					slog.Warn("duplicate text/html part ignored", "message_id", p.MessageID)
				}
			} else if strings.HasPrefix(ct, "text/plain") {
				if p.Text == "" && len(b) > 0 {
					p.Text = string(b)
				} else if len(b) > 0 {
					p.PartErrors++
					slog.Warn("duplicate text/plain part ignored", "message_id", p.MessageID)
				}
			} else {
				filename := ""
				if _, params, err := hdr.ContentType(); err == nil {
					filename = params["name"]
				}
				if filename == "" {
					if cd := hdr.Get("Content-Disposition"); cd != "" {
						for _, part := range strings.Split(cd, ";") {
							part = strings.TrimSpace(part)
							if strings.HasPrefix(strings.ToLower(part), "filename=") {
								filename = strings.Trim(part[9:], "\"")
								break
							}
						}
					}
				}
				cid := strings.Trim(hdr.Get("Content-ID"), "<>")
				p.Attachments = append(p.Attachments, Attachment{
					Filename:    filename,
					ContentType: ct,
					Size:        totalBytes,
					Inline:      true,
					ContentID:   cid,
					Truncated:   truncated,
				})
			}
		case *mail.AttachmentHeader:
			ct, _, _ := hdr.ContentType()
			filename, _ := hdr.Filename()
			// The bytes are read to measure and drain the part; only metadata is
			// stored on the Email row. The original message is kept whole through
			// the storage seam, so re-reading a body from there costs nothing here.
			_, truncated, totalBytes := readLimited(part.Body)
			cid := strings.Trim(hdr.Get("Content-ID"), "<>")
			p.Attachments = append(p.Attachments, Attachment{
				Filename:    filename,
				ContentType: ct,
				Size:        totalBytes,
				Inline:      false,
				ContentID:   cid,
				Truncated:   truncated,
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

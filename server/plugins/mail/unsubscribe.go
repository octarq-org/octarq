package mail

import (
	"bytes"
	netmail "net/mail"
	"strings"
)

// ExtractUnsubscribeURL extracts a valid unsubscribe URL from raw RFC822 MIME bytes.
// It parses the List-Unsubscribe header according to RFC 2369 / RFC 8058,
// preferring HTTP(S) URLs over mailto: targets.
func ExtractUnsubscribeURL(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err == nil {
		if val := msg.Header.Get("List-Unsubscribe"); val != "" {
			return ParseListUnsubscribeHeader(val)
		}
		for k, v := range msg.Header {
			if strings.EqualFold(k, "List-Unsubscribe") && len(v) > 0 {
				return ParseListUnsubscribeHeader(v[0])
			}
		}
	}
	return extractFromRawHeaders(string(raw))
}

func extractFromRawHeaders(rawText string) string {
	lines := strings.Split(rawText, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// End of header section
			break
		}
		if len(trimmed) > 17 && strings.EqualFold(trimmed[:17], "List-Unsubscribe:") {
			return ParseListUnsubscribeHeader(strings.TrimSpace(trimmed[17:]))
		}
	}
	return ""
}

// ParseListUnsubscribeHeader cleans and parses a List-Unsubscribe header value.
// It extracts bracketed or unbracketed URLs, preferring HTTP/HTTPS over mailto.
func ParseListUnsubscribeHeader(hdr string) string {
	cleaned := strings.TrimSpace(hdr)
	if cleaned == "" {
		return ""
	}

	var httpURL string
	var mailtoURL string

	// RFC 2369 items are separated by commas and enclosed in <...>
	parts := strings.Split(cleaned, ",")
	for _, p := range parts {
		item := strings.TrimSpace(p)
		// Extract inside < > if present
		if start := strings.Index(item, "<"); start >= 0 {
			if end := strings.Index(item[start+1:], ">"); end >= 0 {
				item = strings.TrimSpace(item[start+1 : start+1+end])
			}
		}
		item = strings.Trim(item, "<>\"' ")
		lower := strings.ToLower(item)

		if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
			if httpURL == "" {
				httpURL = item
			}
		} else if strings.HasPrefix(lower, "mailto:") {
			if mailtoURL == "" {
				mailtoURL = item
			}
		}
	}

	if httpURL != "" {
		return httpURL
	}
	return mailtoURL
}

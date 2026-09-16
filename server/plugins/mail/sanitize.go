package mail

import (
	"html"
	"regexp"
	"strings"
)

const (
	// MaxEmailContentBytes is the safety threshold for LLM / Agent context windows.
	MaxEmailContentBytes = 4096
)

var (
	// scriptStyleRegex removes <script> and <style> blocks entirely.
	scriptStyleRegex = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)

	// htmlTagRegex matches generic HTML tags.
	htmlTagRegex = regexp.MustCompile(`<[^>]+>`)

	// dataURIRegex matches inline base64 data URIs.
	dataURIRegex = regexp.MustCompile(`data:image/[^;]+;base64,[A-Za-z0-9+/=]+`)

	// rawBase64Regex matches large raw base64 data blobs exceeding 100 characters.
	rawBase64Regex = regexp.MustCompile(`\b[A-Za-z0-9+/=]{120,}\b`)

	// sensitiveQueryTokenRegex redacts security-sensitive tokens in URL query strings.
	sensitiveQueryTokenRegex = regexp.MustCompile(`(?i)(token|reset_token|auth_token|magic_token|access_token|verification_token|password_reset|secret|key|ticket)=([a-zA-Z0-9_\-\.]{12,})`)

	// sensitivePathTokenRegex redacts security-sensitive tokens embedded directly in URL paths.
	sensitivePathTokenRegex = regexp.MustCompile(`(?i)(/(?:reset[-_]password|password[-_]reset|verify[-_]email|magic[-_]link|confirm[-_]email)/)([a-zA-Z0-9_\-\.]{12,})`)

	// multipleNewlinesRegex collapses excessive newlines.
	multipleNewlinesRegex = regexp.MustCompile(`\n{3,}`)
)

// SanitizeEmailContent removes sensitive tokens, strips large base64 inline images,
// and enforces a 4KB ceiling for Agent safety.
func SanitizeEmailContent(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}

	content := raw

	// 1. Redact inline base64 image data
	content = dataURIRegex.ReplaceAllString(content, "[inline image truncated]")

	// 2. Redact long base64 blocks
	content = rawBase64Regex.ReplaceAllString(content, "[base64 data truncated]")

	// 3. Redact sensitive tokens in URLs
	content = sensitiveQueryTokenRegex.ReplaceAllString(content, "$1=[REDACTED_TOKEN]")
	content = sensitivePathTokenRegex.ReplaceAllString(content, "${1}[REDACTED_TOKEN]")

	// 4. Truncate at MaxEmailContentBytes (4KB)
	truncated := false
	if len(content) > MaxEmailContentBytes {
		content = content[:MaxEmailContentBytes] + "\n... [content truncated at 4KB]"
		truncated = true
	}

	return content, truncated
}

// stripHTML cleanly converts HTML text to plain text, stripping tags and decoding entities.
func stripHTML(h string) string {
	if h == "" {
		return ""
	}

	// Remove script and style tags
	s := scriptStyleRegex.ReplaceAllString(h, "")

	// Replace block-level tags with newlines
	blockTags := []string{"</p>", "<p>", "<br>", "<br/>", "<br />", "</div>", "<div>", "</tr>", "</li>", "</h1>", "</h2>", "</h3>"}
	for _, bt := range blockTags {
		s = strings.ReplaceAll(s, bt, "\n")
	}

	// Remove remaining tags
	s = htmlTagRegex.ReplaceAllString(s, "")

	// Decode HTML entities
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")

	// Clean up whitespace
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = multipleNewlinesRegex.ReplaceAllString(s, "\n\n")

	return strings.TrimSpace(s)
}

// GenerateEmailSummary produces a safe, concise overview and detected category for an email.
func GenerateEmailSummary(subject, body, otp string) (string, string) {
	// Detect category
	category := "general"
	lowerSubj := strings.ToLower(subject)
	lowerBody := strings.ToLower(body)
	full := lowerSubj + " " + lowerBody

	if otp != "" || hasOTPContext(full) {
		category = "otp"
	} else if strings.Contains(full, "security") || strings.Contains(full, "password") || strings.Contains(full, "alert") {
		category = "security_alert"
	} else if strings.Contains(full, "invoice") || strings.Contains(full, "bill") || strings.Contains(full, "receipt") || strings.Contains(full, "payment") {
		category = "billing"
	} else if strings.Contains(full, "notification") || strings.Contains(full, "update") {
		category = "notification"
	}

	// Prepare short summary text (up to 300 runes)
	cleanBody, _ := SanitizeEmailContent(body)
	cleanBody = strings.TrimSpace(cleanBody)
	// Collapse multiple spaces/newlines to single line for summary
	cleanBody = strings.Join(strings.Fields(cleanBody), " ")

	runes := []rune(cleanBody)
	summaryText := cleanBody
	if len(runes) > 280 {
		summaryText = string(runes[:280]) + "..."
	}

	if summaryText == "" {
		summaryText = subject
	}

	return summaryText, category
}

package mail

import (
	"regexp"
	"strings"
)

var (
	// keywordPrefixRegex matches a verification keyword followed by optional delimiters and a 4-8 character code.
	keywordPrefixRegex = regexp.MustCompile(`(?i)(?:verification\s+code|verify\s+code|security\s+code|confirmation\s+code|confirm\s+code|one-time\s+password|one-time\s+code|login\s+code|auth(?:entication)?\s+code|access\s+code|activation\s+code|validation\s+code|passcode|pin(?:\s+code)?|otp|验证码|校验码|确认码|动态码|动态口令|授权码|安全码|激活码)[\s:：为是=【\(\[]*(?:is\s+)?([0-9]{3}[-\s][0-9]{3}|[0-9A-Za-z]{4,8})\b`)

	// codeSuffixRegex matches a code followed by "is your [service] verification code" or Chinese equivalent.
	codeSuffixRegex = regexp.MustCompile(`(?i)\b([0-9]{3}[-\s][0-9]{3}|[0-9A-Za-z]{4,8})\b[\s,，.。]*(?:is\s+(?:your\s+)?(?:[a-z0-9_\-\s]{0,20}\s+)?(?:verification|security|confirmation|login|one-time|auth(?:entication)?)\s+code|是(?:[^\s,，。]{0,10})?(?:验证码|校验码|确认码|动态码|动态口令|授权码|安全码|激活码))`)

	// actionVerbRegex matches phrases like "enter 123456 to verify" or "use code 123456".
	actionVerbRegex = regexp.MustCompile(`(?i)(?:enter|use|input|type|copy)\s+(?:code\s+)?([0-9]{3}[-\s][0-9]{3}|[0-9A-Za-z]{4,8})\s+(?:to\s+(?:verify|confirm|log\s*in|authenticate|complete)|as\s+(?:your\s+)?(?:code|verification|otp))`)

	// bracketRegex matches codes enclosed in Chinese brackets like 【123456】.
	bracketRegex = regexp.MustCompile(`[【\[]([0-9]{3}[-\s][0-9]{3}|[0-9A-Za-z]{4,8})[】\]]`)

	// htmlEmphasizedCodeRegex matches emphasized codes in HTML tags (b, strong, h1-h6).
	htmlEmphasizedCodeRegex = regexp.MustCompile(`(?i)<(?:b|strong|h[1-6]|span[^>]*font-size[^>]*)>[\s]*([0-9]{3}[-\s][0-9]{3}|[0-9A-Za-z]{4,8})[\s]*</(?:b|strong|h[1-6]|span)>`)

	// commonWordDenylist rejects common English words that match 4-8 letters without any digits.
	commonWordDenylist = map[string]bool{
		"code": true, "your": true, "from": true, "team": true, "user": true,
		"this": true, "that": true, "click": true, "reset": true, "login": true,
		"email": true, "please": true, "below": true, "valid": true, "enter": true,
		"using": true, "verify": true, "access": true, "secure": true, "account": true,
		"welcome": true, "support": true, "service": true, "message": true, "request": true,
	}
)

// ExtractOTP extracts a 4-8 digit or alphanumeric one-time verification code
// from email subject, plain text body, or HTML body.
func ExtractOTP(subject, text, html string) string {
	// 1. Try Subject first (highest priority and lowest noise)
	if code := extractFromText(subject, true); code != "" {
		return code
	}

	// 2. Prepare plain text body (convert HTML if plain text is empty)
	body := text
	if body == "" && html != "" {
		body = stripHTML(html)
	}

	// 3. Try plain text body
	if code := extractFromText(body, false); code != "" {
		return code
	}

	// 4. Try HTML specific structures (bold/strong tags with context check)
	if html != "" && hasOTPContext(subject+" "+body+" "+html) {
		if code := extractFromHTML(html); code != "" {
			return code
		}
	}

	return ""
}

// extractFromText searches text for OTP candidates using regexes in priority order.
func extractFromText(s string, isSubject bool) string {
	if s == "" {
		return ""
	}

	// Pattern 1: Keyword followed by code
	if m := keywordPrefixRegex.FindStringSubmatch(s); len(m) > 1 {
		if c := cleanAndValidateCode(m[1]); c != "" {
			return c
		}
	}

	// Pattern 2: Code followed by suffix phrase
	if m := codeSuffixRegex.FindStringSubmatch(s); len(m) > 1 {
		if c := cleanAndValidateCode(m[1]); c != "" {
			return c
		}
	}

	// Pattern 3: Action verb pattern (e.g. "enter 123456 to verify")
	if m := actionVerbRegex.FindStringSubmatch(s); len(m) > 1 {
		if c := cleanAndValidateCode(m[1]); c != "" {
			return c
		}
	}

	// Pattern 4: Bracketed code if verification context exists
	if hasOTPContext(s) {
		if m := bracketRegex.FindStringSubmatch(s); len(m) > 1 {
			if c := cleanAndValidateCode(m[1]); c != "" {
				return c
			}
		}
	}

	return ""
}

// extractFromHTML inspects HTML tags that emphasize text (e.g. <b>123456</b>)
func extractFromHTML(h string) string {
	matches := htmlEmphasizedCodeRegex.FindAllStringSubmatch(h, -1)
	for _, m := range matches {
		if len(m) > 1 {
			if c := cleanAndValidateCode(m[1]); c != "" {
				return c
			}
		}
	}
	return ""
}

// cleanAndValidateCode normalizes a matched code and ensures it meets OTP requirements.
func cleanAndValidateCode(raw string) string {
	code := strings.TrimSpace(raw)
	// Normalize hyphenated or spaced 6-digit codes like 123-456 or 123 456
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")

	if len(code) < 4 || len(code) > 8 {
		return ""
	}

	// Check for all-letter words that are in denylist
	lower := strings.ToLower(code)
	if commonWordDenylist[lower] {
		return ""
	}

	// Discard likely years unless there are letters or explicit prefix
	if len(code) == 4 && isAllDigits(code) {
		if code >= "1990" && code <= "2099" {
			// Suspicious year, reject standalone year false positives
			return ""
		}
	}

	return code
}

// hasOTPContext reports whether s contains verification keywords.
func hasOTPContext(s string) bool {
	lower := strings.ToLower(s)
	keywords := []string{
		"verification", "verify", "security code", "confirmation", "confirm",
		"one-time", "passcode", "otp", "auth", "login code", "access code",
		"验证码", "校验码", "确认码", "动态码", "动态口令", "授权码", "安全码", "激活码",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

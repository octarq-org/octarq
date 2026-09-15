package mail

import (
	"strings"
	"testing"
)

func TestExtractOTP_Patterns(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		text    string
		html    string
		want    string
	}{
		{
			name:    "English subject with 6-digit code",
			subject: "Your GitHub authentication code is 123456",
			want:    "123456",
		},
		{
			name:    "Chinese subject with 6-digit code",
			subject: "您的验证码是 482019",
			want:    "482019",
		},
		{
			name:    "Colon and space in subject",
			subject: "[AWS] Verification code: 987654",
			want:    "987654",
		},
		{
			name:    "Chinese brackets and suffix in subject",
			subject: "【Acme】您的动态验证码为: 839201，请勿泄露",
			want:    "839201",
		},
		{
			name:    "Security code keyword",
			subject: "Security code: 736184",
			want:    "736184",
		},
		{
			name:    "5-digit login code",
			subject: "Your login code is 84920",
			want:    "84920",
		},
		{
			name:    "Alphanumeric confirmation code",
			subject: "Confirmation Code: A9F2B8",
			want:    "A9F2B8",
		},
		{
			name:    "Action verb phrase in text",
			subject: "Verify your email",
			text:    "Please use code 9482 to verify your account.",
			want:    "9482",
		},
		{
			name:    "Code followed by is your code",
			subject: "Discord Login",
			text:    "123456 is your Discord verification code.",
			want:    "123456",
		},
		{
			name:    "Code followed by Chinese suffix",
			subject: "登录提醒",
			text:    "839201 是您的验证码，5分钟内有效。",
			want:    "839201",
		},
		{
			name:    "Hyphenated 6-digit code",
			subject: "Login Verification",
			text:    "Your verification code is 123-456",
			want:    "123456",
		},
		{
			name:    "Spaced 6-digit code",
			subject: "Security Alert",
			text:    "Your security code is 789 012",
			want:    "789012",
		},
		{
			name:    "HTML bold tag with OTP context",
			subject: "Your AWS Invoice Verification",
			html:    "<p>Your verification code is <b>987654</b>.</p>",
			want:    "987654",
		},
		{
			name:    "HTML strong tag with OTP context",
			subject: "Password Reset Verification",
			html:    "<div>Security confirmation: <strong>849201</strong></div>",
			want:    "849201",
		},
		{
			name:    "Bracketed code in body with Chinese context",
			subject: "帐号安全通知",
			text:    "请使用【583920】完成动态验证码验证。",
			want:    "583920",
		},
		{
			name:    "One-time password keyword",
			subject: "OTP Login",
			text:    "Your one-time password is 654321.",
			want:    "654321",
		},
		{
			name:    "False positive rejection: Year without code prefix",
			subject: "Annual Review 2025",
			text:    "Welcome to the annual review for 2025.",
			want:    "",
		},
		{
			name:    "False positive rejection: Common words",
			subject: "Please verify your account",
			text:    "Click below to login to your account.",
			want:    "",
		},
		{
			name:    "False positive rejection: Too short or too long",
			subject: "Your code",
			text:    "Code: 12 and Code: 1234567890123",
			want:    "",
		},
		{
			name:    "Empty inputs",
			subject: "",
			text:    "",
			html:    "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractOTP(tt.subject, tt.text, tt.html)
			if got != tt.want {
				t.Errorf("ExtractOTP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeEmailContent_And_Summary(t *testing.T) {
	// 1. Test inline base64 image stripping
	rawWithBase64 := `Welcome! Here is your avatar: data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg== Enjoy!`
	sanitized, truncated := SanitizeEmailContent(rawWithBase64)
	if strings.Contains(sanitized, "iVBORw0KGgoAAAANSUhEUg") {
		t.Errorf("base64 data was not stripped: %s", sanitized)
	}
	if !strings.Contains(sanitized, "[inline image truncated]") {
		t.Errorf("expected [inline image truncated], got: %s", sanitized)
	}
	if truncated {
		t.Errorf("short content should not be truncated")
	}

	// 2. Test sensitive query token redaction
	urlQuery := `Please reset your password at https://auth.example.com/reset-password?token=abcdef1234567890xyz&uid=42`
	sanitizedURL, _ := SanitizeEmailContent(urlQuery)
	if strings.Contains(sanitizedURL, "abcdef1234567890xyz") {
		t.Errorf("sensitive token in query was not redacted: %s", sanitizedURL)
	}
	if !strings.Contains(sanitizedURL, "token=[REDACTED_TOKEN]") {
		t.Errorf("expected token=[REDACTED_TOKEN], got: %s", sanitizedURL)
	}

	// 3. Test sensitive path token redaction
	urlPath := `Click https://auth.example.com/password_reset/abcdef1234567890xyz123 to proceed.`
	sanitizedPath, _ := SanitizeEmailContent(urlPath)
	if strings.Contains(sanitizedPath, "abcdef1234567890xyz123") {
		t.Errorf("sensitive token in path was not redacted: %s", sanitizedPath)
	}
	if !strings.Contains(sanitizedPath, "/password_reset/[REDACTED_TOKEN]") {
		t.Errorf("expected /password_reset/[REDACTED_TOKEN], got: %s", sanitizedPath)
	}

	// 4. Test raw base64 data block truncation
	rawBase64Block := strings.Repeat("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xtbm9wcXJzdHV2d3h5ejAxMjM0NTY3ODk=", 3)
	sanitizedB64, _ := SanitizeEmailContent("Data: " + rawBase64Block)
	if !strings.Contains(sanitizedB64, "[base64 data truncated]") {
		t.Errorf("expected [base64 data truncated], got: %s", sanitizedB64)
	}

	// 5. Test 4KB truncation with natural text
	hugeText := strings.Repeat("This is a long test sentence designed to verify context truncation limits safely. ", 100)
	sanitizedHuge, isTruncated := SanitizeEmailContent(hugeText)
	if !isTruncated {
		t.Errorf("expected isTruncated = true")
	}
	if !strings.Contains(sanitizedHuge, "[content truncated at 4KB]") {
		t.Errorf("missing truncation notice: %s", sanitizedHuge)
	}
	if len(sanitizedHuge) > MaxEmailContentBytes+100 {
		t.Errorf("content too large after truncation: %d", len(sanitizedHuge))
	}

	// 5. Test empty
	emptySan, emptyTrunc := SanitizeEmailContent("")
	if emptySan != "" || emptyTrunc {
		t.Errorf("empty input should return empty, not truncated")
	}

	// 6. Test stripHTML
	htmlInput := `<html><head><style>.a{color:red;}</style><script>alert(1);</script></head><body><h1>Hello</h1><p>World&nbsp;&amp;&nbsp;Friends</p></body></html>`
	plain := stripHTML(htmlInput)
	if strings.Contains(plain, "color:red") || strings.Contains(plain, "alert(1)") {
		t.Errorf("style or script was not stripped: %s", plain)
	}
	if !strings.Contains(plain, "Hello") || !strings.Contains(plain, "World & Friends") {
		t.Errorf("plain text missing content: %s", plain)
	}
	if stripHTML("") != "" {
		t.Errorf("stripHTML empty should return empty")
	}

	// 7. Test GenerateEmailSummary categories
	testsCat := []struct {
		subject string
		body    string
		otp     string
		wantCat string
	}{
		{"Your code", "Body here", "123456", "otp"},
		{"Security Alert: New Login", "Someone logged in from IP", "", "security_alert"},
		{"Your monthly invoice", "Invoice #102 is ready for payment", "", "billing"},
		{"Service Notification", "Scheduled maintenance update", "", "notification"},
		{"Hello from friend", "Catching up on weekend plans", "", "general"},
	}
	for _, tc := range testsCat {
		sum, cat := GenerateEmailSummary(tc.subject, tc.body, tc.otp)
		if cat != tc.wantCat {
			t.Errorf("subject %q: got category %q, want %q", tc.subject, cat, tc.wantCat)
		}
		if sum == "" {
			t.Errorf("summary should not be empty")
		}
	}

	// Test long body summary truncation
	longBody := strings.Repeat("Long sentence describing the event. ", 30)
	sumLong, _ := GenerateEmailSummary("Long email", longBody, "")
	if !strings.HasSuffix(sumLong, "...") {
		t.Errorf("expected summary to be truncated with ..., got: %s", sumLong)
	}
}

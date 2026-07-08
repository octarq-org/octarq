package api

import "testing"

func TestNormalizeTarget(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		// Bare host defaults to https.
		{"example.com", "https://example.com", true},
		{"example.com/path?q=1", "https://example.com/path?q=1", true},
		// Explicit http(s) preserved.
		{"http://example.com", "http://example.com", true},
		{"https://example.com/x", "https://example.com/x", true},
		// Dangerous schemes rejected — these must never reach a 302 Location.
		{"javascript://alert(1)", "", false},
		{"javascript:alert(1)", "", false},
		{"data:text/html,<script>alert(1)</script>", "", false},
		{"vbscript://x", "", false},
		{"file:///etc/passwd", "", false},
		// Malformed / missing host.
		{"https://", "", false},
		{"http://", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeTarget(c.in)
		if ok != c.wantOK || got != c.want {
			t.Errorf("normalizeTarget(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

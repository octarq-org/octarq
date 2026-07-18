package links

import (
	"testing"
)

func TestValidateRedirectTargetsRoutingRules(t *testing.T) {
	bad := &Link{
		Target: "https://ok.example",
		RoutingRules: RoutingRules{
			{Type: "geo", Match: "us", Target: "javascript:alert(1)"},
		},
	}
	if err := validateRedirectTargets(bad); err == nil {
		t.Fatal("expected javascript: routing-rule target to be rejected")
	}

	good := &Link{
		ExpiredURL: "exp.example",
		RoutingRules: RoutingRules{
			{Type: "geo", Match: "us", Target: "target.example/path"},
		},
	}
	if err := validateRedirectTargets(good); err != nil {
		t.Fatalf("expected valid targets to pass, got %v", err)
	}
	if good.ExpiredURL != "https://exp.example" {
		t.Fatalf("expiredUrl not normalized: %q", good.ExpiredURL)
	}
	if good.RoutingRules[0].Target != "https://target.example/path" {
		t.Fatalf("routing target not normalized: %q", good.RoutingRules[0].Target)
	}
}

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

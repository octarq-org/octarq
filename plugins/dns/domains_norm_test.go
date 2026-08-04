package dns

import (
	"errors"
	"testing"
)

func TestNormalizeHostUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"  HTTPS://MYDOMAIN.COM/path/to/page:8080. ", "mydomain.com"},
		{"http://sub.domain.com./", "sub.domain.com"},
		{"example.com:443", "example.com"},
		{"EXAMPLE.COM.", "example.com"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		got := normalizeHost(tt.input)
		if got != tt.want {
			t.Errorf("normalizeHost(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeHostsUnit(t *testing.T) {
	t.Parallel()

	tTrue := true
	fFalse := false

	entries := []hostEntry{
		{Host: "  HTTPS://A.COM/path ", Enabled: &tTrue},
		{Host: "a.com", Enabled: &fFalse}, // duplicate, should be skipped
		{Host: "B.COM:80", Enabled: &fFalse},
		{Host: "   ", Enabled: &tTrue}, // empty, skipped
	}

	out := normalizeHosts(entries)
	if len(out) != 2 {
		t.Fatalf("expected 2 host entries, got %d", len(out))
	}

	if out[0].Host != "a.com" || !out[0].Enabled {
		t.Errorf("entry 0 mismatch: %+v", out[0])
	}
	if out[1].Host != "b.com" || out[1].Enabled {
		t.Errorf("entry 1 mismatch: %+v", out[1])
	}
}

func TestProviderErrUnit(t *testing.T) {
	t.Parallel()

	p := &Plugin{}

	// "upstream timeout" has no auth/auth/invalid/exists keywords → falls through
	// to the default bucket. The response must NOT contain the raw provider text,
	// but providerErr still returns a non-nil error.
	err := p.providerErr("create domain", errors.New("upstream timeout"))
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	raw := "upstream timeout"
	if msg := err.Error(); msg == "create domain: "+raw {
		t.Errorf("provider raw text leaked into response: %v", msg)
	}
	// The sanitised response must still include the action name.
	if msg := err.Error(); len(msg) == 0 || msg == "create domain: " {
		t.Errorf("providerErr returned empty message: %v", msg)
	}

	// Auth-related errors are classified as a credential hint.
	errAuth := p.providerErr("list zones", errors.New("401 Unauthorized: invalid token xyz-secret-abc"))
	if errAuth == nil {
		t.Fatal("expected non-nil error for auth failure")
	}
	if msg := errAuth.Error(); msg != "list zones: authentication failed — check your API token or credentials" {
		t.Errorf("unexpected auth error message: %v", msg)
	}
	// The raw secret must not appear in the response.
	if msg := errAuth.Error(); len(msg) > 0 {
		for _, leaked := range []string{"xyz-secret-abc", "invalid token"} {
			if contains(msg, leaked) {
				t.Errorf("provider raw text %q leaked into response: %v", leaked, msg)
			}
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

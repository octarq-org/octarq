package api

import (
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/config"
)

// The download link for a full export is a signed token. A token minted for
// no workspace would address an export that does not exist.
func TestExportSignedTokenRefusesAnEmptyWorkspace(t *testing.T) {
	h := &Handler{cfg: &config.Config{SecretKey: strings.Repeat("k", 32)}}
	if _, err := h.GenerateExportSignedToken(0, time.Minute); err == nil {
		t.Fatal("token for org 0: got no error")
	}
}

func TestExportSignedTokenAndURL(t *testing.T) {
	h := &Handler{cfg: &config.Config{SecretKey: strings.Repeat("k", 32)}}

	// A zero lifetime must still mint a usable token.
	tok, err := h.GenerateExportSignedToken(5, 0)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if tok == "" {
		t.Fatal("token with zero lifetime is empty")
	}

	link, err := h.GenerateExportSignedURL(5)
	if err != nil {
		t.Fatalf("url: %v", err)
	}
	if !strings.HasPrefix(link, "/api/account/export?token=") {
		t.Fatalf("url = %q, want the export path with a token", link)
	}
}

// Export files carry conversation content, so credentials in that content must
// leave the instance masked.
func TestSanitizeSecretMasksCredentials(t *testing.T) {
	openAIStyle := "sk-" + strings.Repeat("a", 24)
	githubStyle := "ghp_" + strings.Repeat("b", 20)
	bearerStyle := "Bearer " + strings.Repeat("c", 24)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"provider key", "key " + openAIStyle + " here", "key sk-**** here"},
		{"github token", "token " + githubStyle + " ok", "token [redacted] ok"},
		{"bearer header", "Authorization: " + bearerStyle, "Authorization: [redacted]"},
		{"plain text", "nothing to hide", "nothing to hide"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeSecret(tc.in); got != tc.want {
				t.Fatalf("sanitizeSecret(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

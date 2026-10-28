package mail

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// TestSenderFromCredentialRejectsIncompleteConfig pins the fail-closed rules of
// the Integrations Hub path. A sender without a From address or without a
// relay would either be rejected by the relay or send under a domain the
// operator does not control, so every gap is an error here.
func TestSenderFromCredentialRejectsIncompleteConfig(t *testing.T) {
	p := New()

	cases := []struct {
		name string
		cred *plugin.Credential
		want string
	}{
		{"nil credential", nil, "empty credential configuration"},
		{"empty config", &plugin.Credential{ID: 1}, "empty credential configuration"},
		{"malformed config", &plugin.Credential{
			ID: 1, ConfigJSON: json.RawMessage(`{"fromEmail":`),
		}, "malformed credential configuration"},
		{"missing from", &plugin.Credential{
			ID: 1, Provider: "smtp", ConfigJSON: json.RawMessage(`{"host":"smtp.example.com"}`),
		}, "sender From address is not configured"},
		{"resend without key", &plugin.Credential{
			ID: 1, Provider: "resend", ConfigJSON: json.RawMessage(`{"fromEmail":"a@example.com"}`),
		}, "resend API key is required"},
		{"smtp without host", &plugin.Credential{
			ID: 1, Provider: "smtp", ConfigJSON: json.RawMessage(`{"fromEmail":"a@example.com"}`),
		}, "smtp host is required"},
	}

	for _, tc := range cases {
		if _, _, err := p.senderFromCredential(tc.cred); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: expected %q, got %v", tc.name, tc.want, err)
		}
	}
}

// TestSenderFromCredentialAcceptsAliases pins the accepted key spellings, so a
// credential saved by an older helper still sends.
func TestSenderFromCredentialAcceptsAliases(t *testing.T) {
	p := New()

	// Resend accepts apiKey or secretKey.
	for _, key := range []string{"apiKey", "secretKey"} {
		cred := &plugin.Credential{
			ID: 1, Provider: "resend",
			ConfigJSON: json.RawMessage(`{"fromEmail":"a@example.com","` + key + `":"re-123"}`),
		}
		sender, from, err := p.senderFromCredential(cred)
		if err != nil || sender == nil || from != "a@example.com" {
			t.Errorf("resend via %s: expected a sender, got %v / %q", key, err, from)
		}
	}

	// SMTP accepts host or smtpHost, user or apiKey, pass or secretKey, and
	// defaults the port.
	cred := &plugin.Credential{
		ID: 1, Provider: "smtp",
		ConfigJSON: json.RawMessage(`{"smtpHost":"smtp.example.com","apiKey":"u","secretKey":"p","fromEmail":"a@example.com"}`),
	}
	sender, from, err := p.senderFromCredential(cred)
	if err != nil || sender == nil || from != "a@example.com" {
		t.Fatalf("smtp aliases: expected a sender, got %v / %q", err, from)
	}
}

package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	internalmail "github.com/octarq-org/octarq/server/internal/mail"
	"github.com/octarq-org/octarq/server/plugin"
)

// buildSendInput constructs a send request for the given registry and ID.
// The two registries number their rows independently, so the source is part of
// the identity: a bare ID is ambiguous.
func buildSendInput(ctx huma.Context, id uint, source string) *SendEmailInput {
	return &SendEmailInput{
		Ctx: ctx,
		Body: struct {
			internalmail.Message
			SMTPSenderID     uint   `json:"smtpSenderId"`
			SMTPSenderSource string `json:"smtpSenderSource"`
			TrackLinks       bool   `json:"trackLinks"`
		}{
			SMTPSenderID:     id,
			SMTPSenderSource: source,
			Message:          internalmail.Message{To: []string{"to@example.com"}, Subject: "s", Text: "b"},
		},
	}
}

func sendReq(t *testing.T) huma.Context {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	req.Header.Set("X-Org-ID", "1")
	return humago.NewContext(nil, req, httptest.NewRecorder())
}

func TestSendEmailSourceScoping(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()
	p.publishEvent = func(uint, string, any) {}
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }

	localHost, localPort, localGot := captureSMTP(t)
	local := SMTPSender{OrgID: 1, Name: "local relay", Host: localHost, Port: localPort, User: "u", Pass: "pw", FromEmail: "local@example.com"}
	if err := p.db.Create(&local).Error; err != nil {
		t.Fatalf("seed local sender: %v", err)
	}

	hubHost, hubPort, hubGot := captureSMTP(t)
	var hubCalls int
	p.host = &plugin.TestHost{
		LookupCredentialMock: func(ctx context.Context, orgID uint, query plugin.CredentialQuery) (*plugin.Credential, error) {
			hubCalls++
			if orgID != 1 || query.ProviderType != "communication" || query.ID == 0 {
				return nil, plugin.ErrCredentialNotFound
			}
			if query.ID == local.ID {
				cfg, _ := json.Marshal(map[string]any{
					"host":      hubHost,
					"port":      hubPort,
					"user":      "hub-user",
					"fromEmail": "hub@example.com",
				})
				return &plugin.Credential{
					ID:           query.ID,
					OrgID:        1,
					ProviderType: "communication",
					Provider:     "smtp",
					ConfigJSON:   cfg,
				}, nil
			}
			return nil, plugin.ErrCredentialNotFound
		},
	}

	// A local source must send through the local row and never consult the hub,
	// even when the hub has an integration with the same ID.
	if _, err := p.sendEmail(ctx, buildSendInput(sendReq(t), local.ID, "local")); err != nil {
		t.Fatalf("local send: %v", err)
	}
	if hubCalls != 0 {
		t.Errorf("hub lookups for a local send = %d, want 0", hubCalls)
	}
	if !strings.Contains(localGot(), "local@example.com") {
		t.Errorf("local relay did not receive the message:\n%s", localGot())
	}

	// An integration source must send through the hub and never read the local
	// row that shares the ID.
	if _, err := p.sendEmail(ctx, buildSendInput(sendReq(t), local.ID, "integration")); err != nil {
		t.Fatalf("integration send: %v", err)
	}
	if hubCalls != 1 {
		t.Errorf("hub lookups = %d, want 1", hubCalls)
	}
	if !strings.Contains(hubGot(), "hub@example.com") {
		t.Errorf("hub relay did not receive the message:\n%s", hubGot())
	}

	// An absent integration must not fall back to a local row.
	orphanHost, orphanPort, orphanGot := captureSMTP(t)
	if err := p.db.Create(&SMTPSender{OrgID: 1, Name: "orphan", Host: orphanHost, Port: orphanPort, User: "u", Pass: "pw", FromEmail: "orphan@example.com"}).Error; err != nil {
		t.Fatalf("seed orphan sender: %v", err)
	}
	var orphan SMTPSender
	if err := p.db.Where("name = ?", "orphan").First(&orphan).Error; err != nil {
		t.Fatalf("read orphan: %v", err)
	}
	if _, err := p.sendEmail(ctx, buildSendInput(sendReq(t), orphan.ID, "integration")); err == nil {
		t.Fatal("expected an error for an absent integration ID")
	}
	if strings.Contains(orphanGot(), "orphan@example.com") {
		t.Error("an absent integration must not fall back to the local sender table")
	}

	// A missing or unknown source is refused before any registry is consulted.
	before := hubCalls
	for _, source := range []string{"", "hub"} {
		if _, err := p.sendEmail(ctx, buildSendInput(sendReq(t), local.ID, source)); err == nil {
			t.Errorf("source %q must be refused", source)
		}
	}
	if hubCalls != before {
		t.Errorf("refused requests reached the hub: lookups %d -> %d", before, hubCalls)
	}
}

func TestSendEmailIntegrationRejectsNonMailProvider(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	var calls int
	p.host = &plugin.TestHost{
		LookupCredentialMock: func(ctx context.Context, orgID uint, query plugin.CredentialQuery) (*plugin.Credential, error) {
			calls++
			return &plugin.Credential{
				ID:           query.ID,
				OrgID:        orgID,
				ProviderType: "communication",
				Provider:     "telegram",
				ConfigJSON:   json.RawMessage(`{"botToken":"x"}`),
			}, nil
		},
	}

	if _, err := p.sendEmail(ctx, buildSendInput(sendReq(t), 5, "integration")); err == nil {
		t.Fatal("a Telegram integration must not be used as a mail sender")
	}
	if calls != 1 {
		t.Errorf("hub lookups = %d, want 1", calls)
	}
}

func TestSendEmailIntegrationRequiresHub(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()
	p.host = nil

	if _, err := p.sendEmail(ctx, buildSendInput(sendReq(t), 5, "integration")); err == nil {
		t.Fatal("an integration source must fail closed when no hub is mounted")
	}
}

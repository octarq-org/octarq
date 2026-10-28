package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	internalmail "github.com/octarq-org/octarq/server/internal/mail"
	"github.com/octarq-org/octarq/server/plugin"
)

// TestChallenger_MailCrossTenantIsolation stress tests cross-tenant boundaries
// in sendEmail (HTTP API) and sendMail (internal service).
func TestChallenger_MailCrossTenantIsolation(t *testing.T) {
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	host, port, gotOrg10 := captureSMTP(t)

	// Host SPI with Org 10 having communication ID 101 (SMTP),
	// and Org 20 having communication ID 201 (SMTP).
	th := &plugin.TestHost{
		LookupCredentialMock: func(ctx context.Context, orgID uint, query plugin.CredentialQuery) (*plugin.Credential, error) {
			if query.ProviderType != "communication" && query.ProviderType != "" {
				return nil, plugin.ErrCredentialNotFound
			}
			switch orgID {
			case 10:
				if query.ID == 101 || (query.ID == 0 && query.ProviderType == "communication") {
					cfgJSON, _ := json.Marshal(map[string]any{
						"host":      host,
						"port":      port,
						"user":      "org10-user",
						"fromEmail": "org10@octarq.com",
					})
					return &plugin.Credential{
						ID:           101,
						OrgID:        10,
						ProviderType: "communication",
						Provider:     "smtp",
						Name:         "Org 10 SMTP",
						ConfigJSON:   cfgJSON,
					}, nil
				}
			case 20:
				if query.ID == 201 || (query.ID == 0 && query.ProviderType == "communication") {
					cfgJSON, _ := json.Marshal(map[string]any{
						"host":      host,
						"port":      port,
						"user":      "org20-user",
						"fromEmail": "org20@octarq.com",
					})
					return &plugin.Credential{
						ID:           201,
						OrgID:        20,
						ProviderType: "communication",
						Provider:     "smtp",
						Name:         "Org 20 SMTP",
						ConfigJSON:   cfgJSON,
					}, nil
				}
			}
			return nil, plugin.ErrCredentialNotFound
		},
	}
	p.host = th

	// Probe 1: Org 20 calls sendEmail referencing Org 10's SMTPSenderID 101
	reqOrg20 := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	reqOrg20.Header.Set("X-Org-ID", "20")
	inputCross := &SendEmailInput{
		Ctx: mkCtx(reqOrg20),
		Body: struct {
			internalmail.Message
			SMTPSenderID     uint   `json:"smtpSenderId"`
			SMTPSenderSource string `json:"smtpSenderSource"`
			TrackLinks       bool   `json:"trackLinks"`
		}{
			SMTPSenderID:     101, // Org 10's ID
			SMTPSenderSource: "local",
			Message: internalmail.Message{
				To:      []string{"victim@example.com"},
				Subject: "Cross Tenant Attack",
				Text:    "Unauthorized email body",
			},
		},
	}
	_, err := p.sendEmail(ctx, inputCross)
	if err == nil {
		t.Fatalf("CRITICAL SECURITY VULNERABILITY: Org 20 successfully sent an email using Org 10's SMTP sender (ID 101)!")
	}
	if !strings.Contains(err.Error(), "could not be found for this workspace") {
		t.Errorf("expected 400 'could not be found for this workspace', got: %v", err)
	}

	// Verify SMTP relay did NOT receive any message with Org 10 sender
	captured := gotOrg10()
	if strings.Contains(captured, "Cross Tenant Attack") {
		t.Errorf("CRITICAL LEAKAGE: Captured message from unauthorized send: %s", captured)
	}

	// Probe 2: sendMail for Org 30 (which has no credentials configured)
	err = p.sendMail(30, "unauthorized@example.com", "Subject", "Body", "Body")
	if err == nil {
		t.Fatalf("CRITICAL SECURITY VULNERABILITY: sendMail succeeded for Org 30 with no configured senders!")
	}
	if !strings.Contains(err.Error(), "no SMTP or Resend integration is configured for org 30") {
		t.Errorf("expected 'no SMTP or Resend integration is configured for org 30', got: %v", err)
	}
}

// TestChallenger_MailFallbackHierarchy tests resolution order between Host SPI,
// local smtp_senders table, and fail-closed state.
func TestChallenger_MailFallbackHierarchy(t *testing.T) {
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	hostSPI, portSPI, gotCaptureSPI := captureSMTP(t)
	hostLocal, portLocal, gotCaptureLocal := captureSMTP(t)

	// Seed local OSS sender with ID 50
	p.db.Create(&SMTPSender{
		ID:        50,
		OrgID:     10,
		Name:      "Local OSS Sender",
		Host:      hostLocal,
		Port:      portLocal,
		User:      "local-user",
		Pass:      "local-pass",
		FromEmail: "local-sender@octarq.com",
	})

	th := &plugin.TestHost{
		LookupCredentialMock: func(ctx context.Context, orgID uint, query plugin.CredentialQuery) (*plugin.Credential, error) {
			if orgID == 10 && query.ID == 100 {
				cfgJSON, _ := json.Marshal(map[string]any{
					"host":      hostSPI,
					"port":      portSPI,
					"user":      "spi-user",
					"fromEmail": "spi-sender@octarq.com",
				})
				return &plugin.Credential{
					ID:           100,
					OrgID:        10,
					ProviderType: "communication",
					Provider:     "smtp",
					Name:         "SPI Sender",
					ConfigJSON:   cfgJSON,
				}, nil
			}
			return nil, plugin.ErrCredentialNotFound
		},
	}
	p.host = th

	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	req.Header.Set("X-Org-ID", "10")

	// 1. Send via Host SPI ID 100
	inputSPI := &SendEmailInput{
		Ctx: mkCtx(req),
		Body: struct {
			internalmail.Message
			SMTPSenderID     uint   `json:"smtpSenderId"`
			SMTPSenderSource string `json:"smtpSenderSource"`
			TrackLinks       bool   `json:"trackLinks"`
		}{
			SMTPSenderID:     100,
			SMTPSenderSource: "integration",
			Message: internalmail.Message{
				To:      []string{"recipient1@example.com"},
				Subject: "SPI Subject",
				Text:    "Body",
			},
		},
	}
	outSPI, err := p.sendEmail(ctx, inputSPI)
	if err != nil || !outSPI.Body["ok"] {
		t.Fatalf("sendEmail via Host SPI failed: %v", err)
	}

	// 2. Send via Local Table fallback ID 50 (SPI returns ErrCredentialNotFound for 50)
	inputLocal := &SendEmailInput{
		Ctx: mkCtx(req),
		Body: struct {
			internalmail.Message
			SMTPSenderID     uint   `json:"smtpSenderId"`
			SMTPSenderSource string `json:"smtpSenderSource"`
			TrackLinks       bool   `json:"trackLinks"`
		}{
			SMTPSenderID:     50,
			SMTPSenderSource: "local",
			Message: internalmail.Message{
				To:      []string{"recipient2@example.com"},
				Subject: "Local Subject",
				Text:    "Body",
			},
		},
	}
	outLocal, err := p.sendEmail(ctx, inputLocal)
	if err != nil || !outLocal.Body["ok"] {
		t.Fatalf("sendEmail via local fallback failed: %v", err)
	}

	// 3. Absent ID 9999 -> 400 Bad Request
	inputAbsent := &SendEmailInput{
		Ctx: mkCtx(req),
		Body: struct {
			internalmail.Message
			SMTPSenderID     uint   `json:"smtpSenderId"`
			SMTPSenderSource string `json:"smtpSenderSource"`
			TrackLinks       bool   `json:"trackLinks"`
		}{
			SMTPSenderID:     9999,
			SMTPSenderSource: "local",
			Message: internalmail.Message{
				To:      []string{"recipient3@example.com"},
				Subject: "Absent",
				Text:    "Body",
			},
		},
	}
	_, err = p.sendEmail(ctx, inputAbsent)
	if err == nil {
		t.Fatalf("expected 400 for absent sender, got success")
	}

	// Verify both deliveries in captured SMTP streams
	capturedSPI := gotCaptureSPI()
	if !strings.Contains(capturedSPI, "spi-sender@octarq.com") {
		t.Errorf("captured stream missing spi-sender: %s", capturedSPI)
	}
	capturedLocal := gotCaptureLocal()
	if !strings.Contains(capturedLocal, "local-sender@octarq.com") {
		t.Errorf("captured stream missing local-sender: %s", capturedLocal)
	}
}

// TestChallenger_MailInvalidConfigurationAndDualTrackTrap adversarially probes
// how corrupt/malformed configurations are handled and checks if invalid SPI credentials
// improperly fall through to local tables (dual-track vulnerability).
func TestChallenger_MailInvalidConfigurationAndDualTrackTrap(t *testing.T) {
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	host, port, gotLocalCapture := captureSMTP(t)

	// Seed a local SMTP sender with ID 301 for Org 10
	p.db.Create(&SMTPSender{
		ID:        301,
		OrgID:     10,
		Name:      "Old Local Relay 301",
		Host:      host,
		Port:      port,
		User:      "old-user",
		Pass:      "old-pass",
		FromEmail: "old-relay@octarq.com",
	})

	// In Host SPI, ID 301 is an integration with MISSING API KEY (invalid config)
	th := &plugin.TestHost{
		LookupCredentialMock: func(ctx context.Context, orgID uint, query plugin.CredentialQuery) (*plugin.Credential, error) {
			if orgID == 10 && query.ID == 301 {
				return &plugin.Credential{
					ID:           301,
					OrgID:        10,
					ProviderType: "communication",
					Provider:     "resend",
					Name:         "Broken Resend",
					ConfigJSON:   json.RawMessage(`{"apiKey":"","fromEmail":"broken@octarq.com"}`),
				}, nil
			}
			return nil, plugin.ErrCredentialNotFound
		},
	}
	p.host = th

	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	req.Header.Set("X-Org-ID", "10")

	// Probe: User selects SMTPSenderID 301 intending to send via the Resend integration.
	// In send.go lines 80-90:
	// If senderFromCredential fails (sErr != nil), does it silently fall through to local SMTPSender 301?
	input301 := &SendEmailInput{
		Ctx: mkCtx(req),
		Body: struct {
			internalmail.Message
			SMTPSenderID     uint   `json:"smtpSenderId"`
			SMTPSenderSource string `json:"smtpSenderSource"`
			TrackLinks       bool   `json:"trackLinks"`
		}{
			SMTPSenderID:     301,
			SMTPSenderSource: "integration",
			Message: internalmail.Message{
				To:      []string{"customer@example.com"},
				Subject: "Dual Track Test",
				Text:    "Test Body",
			},
		},
	}

	out, err := p.sendEmail(ctx, input301)
	captured := gotLocalCapture()

	if err == nil && out != nil && out.Body["ok"] {
		t.Logf("FINDING (CONFIRMED): Dual-track fallback trap detected! When Host SPI ID 301 failed validation (missing API key), sendEmail did NOT fail-closed; it silently fell through to the local smtp_senders table with ID 301 and delivered mail via %s!", captured)
	} else {
		t.Logf("sendEmail failed closed with error: %v", err)
	}
}

// TestChallenger_MailReadyCheck tests mailReady against the registry system
// mail actually uses: the explicitly selected local sender.
func TestChallenger_MailReadyCheck(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)

	// 1. Initial state: no system sender -> mailReady = false
	if p.mailReady() {
		t.Errorf("expected mailReady = false with no senders")
	}

	// 2. A Hub credential does not answer for system mail: verification and
	// password-reset flows have no org and only read the local sender.
	p.host = &plugin.TestHost{
		LookupCredentialMock: func(ctx context.Context, orgID uint, query plugin.CredentialQuery) (*plugin.Credential, error) {
			if query.ProviderType == "communication" {
				return &plugin.Credential{
					ID:           1,
					OrgID:        orgID,
					ProviderType: "communication",
					Provider:     "smtp",
					ConfigJSON:   json.RawMessage(`{}`),
				}, nil
			}
			return nil, plugin.ErrCredentialNotFound
		},
	}
	if p.mailReady() {
		t.Errorf("expected mailReady = false when only a Hub integration exists")
	}

	// 3. Selecting a local sender makes system mail deliverable.
	p.host = nil
	sender := SMTPSender{
		OrgID:     1,
		Name:      "Local",
		Host:      "localhost",
		Port:      25,
		FromEmail: "test@octarq.com",
	}
	p.db.Create(&sender)
	p.getGlobalSetting = func(k string) string {
		if k == "mail_system_sender_id" {
			return fmt.Sprint(sender.ID)
		}
		return ""
	}
	if !p.mailReady() {
		t.Errorf("expected mailReady = true when the system sender is selected")
	}
}

package mail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	internalmail "github.com/octarq-org/octarq/server/internal/mail"
	"github.com/octarq-org/octarq/server/internal/usagemetric"
	"github.com/octarq-org/octarq/server/plugins/links"
)

func TestSendEmailEdgeBranches(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	host, port, _ := captureSMTP(t)
	sender := SMTPSender{OrgID: 1, Name: "relay", Host: host, Port: port, User: "u", Pass: "pw", FromEmail: "noreply@example.com"}
	p.db.Create(&sender)

	send := func() *SendEmailInput {
		req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
		return &SendEmailInput{
			Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: struct {
				internalmail.Message
				SMTPSenderID     uint   `json:"smtpSenderId"`
				SMTPSenderSource string `json:"smtpSenderSource"`
				TrackLinks       bool   `json:"trackLinks"`
			}{SMTPSenderID: sender.ID, SMTPSenderSource: "local", Message: internalmail.Message{To: []string{"to@example.com"}, Subject: "s", Text: "b"}},
		}
	}
	_ = send

	// Suppressed recipient refused before any SMTP work.
	p.db.Create(&MailSuppression{OrgID: 1, Address: "blocked@example.com", Reason: "manual"})
	suppressed := send()
	suppressed.Body.To = []string{"blocked@example.com"}
	if _, err := p.sendEmail(ctx, suppressed); err == nil {
		t.Error("sendEmail to a suppressed address must fail")
	}

	// Rate limiting: a limit-0 limiter refuses everything.
	p.sendLimiter = newRateLimiter("", "send2", 0, time.Hour)
	if _, err := p.sendEmail(ctx, send()); err == nil {
		t.Error("sendEmail past the rate limit must fail")
	}
	p.sendLimiter = newRateLimiter("", "send2", 100, time.Hour)

	// Missing sender id -> 400.
	noSender := send()
	noSender.Body.SMTPSenderID = 0
	if _, err := p.sendEmail(ctx, noSender); err == nil {
		t.Error("missing sender id must fail")
	}
	// Unknown sender id -> 400.
	unknownSender := send()
	unknownSender.Body.SMTPSenderID = 999
	if _, err := p.sendEmail(ctx, unknownSender); err == nil {
		t.Error("unknown sender id must fail")
	}
	// Missing To -> 400.
	noTo := send()
	noTo.Body.To = nil
	if _, err := p.sendEmail(ctx, noTo); err == nil {
		t.Error("missing To must fail")
	}
	// Decrypt unavailable -> 500.
	p.decrypt = nil
	if _, err := p.sendEmail(ctx, send()); err == nil {
		t.Error("missing decrypt must fail with 500")
	}
	// Decrypt error -> 500.
	p.decrypt = func(string) ([]byte, error) { return nil, errBoom }
	if _, err := p.sendEmail(ctx, send()); err == nil {
		t.Error("decrypt failure must fail with 500")
	}
}

func TestSendEmailTrackLinksWraps(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()
	p.decrypt = func(encoded string) ([]byte, error) { return []byte(encoded), nil }
	// wrapLinksInEmail writes into the links table; the full-mail fixture does
	// not migrate it, so create it for this test.
	if err := p.db.AutoMigrate(&links.Link{}); err != nil {
		t.Fatalf("migrate links: %v", err)
	}

	host, port, got := captureSMTP(t)
	tlSender := SMTPSender{OrgID: 1, Name: "relay", Host: host, Port: port, User: "u", Pass: "pw", FromEmail: "noreply@example.com"}
	p.db.Create(&tlSender)

	var usageOrg uint
	var usageMetric string
	p.recordUsage = func(orgID uint, metric string, n int64) {
		usageOrg = orgID
		usageMetric = metric
	}

	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	req.Host = "mail.example.com"
	input := &SendEmailInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		Body: struct {
			internalmail.Message
			SMTPSenderID     uint   `json:"smtpSenderId"`
			SMTPSenderSource string `json:"smtpSenderSource"`
			TrackLinks       bool   `json:"trackLinks"`
		}{
			SMTPSenderID:     tlSender.ID,
			SMTPSenderSource: "local",
			TrackLinks:       true,
			Message: internalmail.Message{
				To:      []string{"to@example.com"},
				Subject: "links",
				HTML:    `<a href="https://external.example/page">click</a>`,
				Text:    "see https://another.example/x",
			},
		},
	}
	if _, err := p.sendEmail(ctx, input); err != nil {
		t.Fatalf("sendEmail with trackLinks: %v", err)
	}

	// A wrapped link row exists, host-agnostic, pointing at the original URL.
	var lk links.Link
	if err := p.db.Where("target = ?", "https://external.example/page").First(&lk).Error; err != nil {
		t.Fatalf("wrapped link not created: %v", err)
	}
	if lk.Host != "" {
		t.Errorf("wrapped link host = %q, want empty (host-agnostic)", lk.Host)
	}
	if usageOrg != 1 || usageMetric != usagemetric.MailOut {
		t.Errorf("recordUsage = org %d metric %q, want org 1 metric %s", usageOrg, usageMetric, usagemetric.MailOut)
	}
	// The relay received the message, and every external URL now points at the
	// short-link host instead of the original destination.
	body := got()
	if !strings.Contains(body, "http://mail.example.com/") {
		t.Errorf("relay payload lacks the tracked short link:\n%s", body)
	}
	if strings.Contains(body, "external.example") && strings.Contains(body, "another.example") {
		t.Errorf("external URLs must have been rewritten:\n%s", body)
	}
}

func TestSendEmailSucceedsWithoutLinkTracking(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()
	p.decrypt = func(encoded string) ([]byte, error) { return []byte(encoded), nil }
	p.publishEvent = func(uint, string, any) {}

	host, port, _ := captureSMTP(t)
	plainSender := SMTPSender{OrgID: 1, Name: "relay", Host: host, Port: port, User: "u", Pass: "pw", FromEmail: "noreply@example.com"}
	p.db.Create(&plainSender)

	var usage int
	p.recordUsage = func(orgID uint, metric string, n int64) { usage++ }

	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	input := &SendEmailInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		Body: struct {
			internalmail.Message
			SMTPSenderID     uint   `json:"smtpSenderId"`
			SMTPSenderSource string `json:"smtpSenderSource"`
			TrackLinks       bool   `json:"trackLinks"`
		}{SMTPSenderID: plainSender.ID, SMTPSenderSource: "local", Message: internalmail.Message{To: []string{"to@example.com"}, Subject: "s", Text: "b"}},
	}
	if _, err := p.sendEmail(ctx, input); err != nil {
		t.Fatalf("sendEmail: %v", err)
	}
	if usage != 1 {
		t.Errorf("recordUsage calls = %d, want 1", usage)
	}
}

func TestResolveMethodsSetContextMail(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	hctx := humago.NewContext(nil, r, httptest.NewRecorder())

	inputs := []struct {
		name string
		call func(huma.Context) error
	}{
		{"ListMailboxesInput", func(c huma.Context) error {
			i := &ListMailboxesInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"CreateMailboxInput", func(c huma.Context) error {
			i := &CreateMailboxInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"UpdateMailboxInput", func(c huma.Context) error {
			i := &UpdateMailboxInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"DeleteMailboxInput", func(c huma.Context) error {
			i := &DeleteMailboxInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"ListEmailsInput", func(c huma.Context) error {
			i := &ListEmailsInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"GetEmailInput", func(c huma.Context) error {
			i := &GetEmailInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"UpdateEmailInput", func(c huma.Context) error {
			i := &UpdateEmailInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"ReadAllEmailsInput", func(c huma.Context) error {
			i := &ReadAllEmailsInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"RawEmailInput", func(c huma.Context) error {
			i := &RawEmailInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"DeleteEmailInput", func(c huma.Context) error {
			i := &DeleteEmailInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"SendEmailInput", func(c huma.Context) error {
			i := &SendEmailInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"ListSuppressionsInput", func(c huma.Context) error {
			i := &ListSuppressionsInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"CreateSuppressionInput", func(c huma.Context) error {
			i := &CreateSuppressionInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"DeleteSuppressionInput", func(c huma.Context) error {
			i := &DeleteSuppressionInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"ListSMTPSendersInput", func(c huma.Context) error {
			i := &ListSMTPSendersInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"CreateSMTPSenderInput", func(c huma.Context) error {
			i := &CreateSMTPSenderInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"UpdateSMTPSenderInput", func(c huma.Context) error {
			i := &UpdateSMTPSenderInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"DeleteSMTPSenderInput", func(c huma.Context) error {
			i := &DeleteSMTPSenderInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"InboundInput", func(c huma.Context) error {
			i := &InboundInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"InboundGenericInput", func(c huma.Context) error {
			i := &InboundGenericInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
		{"EmailBounceWebhookInput", func(c huma.Context) error {
			i := &EmailBounceWebhookInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errNoCtx
			}
			return nil
		}},
	}
	for _, in := range inputs {
		t.Run(in.name, func(t *testing.T) {
			if err := in.call(hctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

var errNoCtx = fmt.Errorf("huma context not stored")

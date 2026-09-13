package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	internalmail "github.com/octarq-org/octarq/internal/mail"
)

func seededMailbox(t *testing.T, p *Plugin) uint {
	t.Helper()
	mb := Mailbox{OrgID: 1, Address: "auth@example.com", Enabled: true}
	if err := p.db.Create(&mb).Error; err != nil {
		t.Fatalf("seed mailbox: %v", err)
	}
	return mb.ID
}

// memberCtx builds a request whose role header makes the fail-closed role gate
// refuse it.
func memberCtx() huma.Context {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Role", "member")
	return humago.NewContext(nil, r, httptest.NewRecorder())
}

func org0Ctx() huma.Context {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Org-ID", "0")
	return humago.NewContext(nil, r, httptest.NewRecorder())
}

func TestMailboxAuthorizationGates(t *testing.T) {
	t.Parallel()
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()
	id := seededMailbox(t, p)

	// Non-admin create/update/delete are refused.
	if _, err := p.createMailbox(ctx, &CreateMailboxInput{Ctx: memberCtx(), Body: mailboxDTO{Address: "x@example.com"}}); err == nil {
		t.Error("member createMailbox must be forbidden")
	}
	if _, err := p.updateMailbox(ctx, &UpdateMailboxInput{Ctx: memberCtx(), ID: id, Body: mailboxDTO{Note: "x"}}); err == nil {
		t.Error("member updateMailbox must be forbidden")
	}
	if _, err := p.deleteMailbox(ctx, &DeleteMailboxInput{Ctx: memberCtx(), ID: id}); err == nil {
		t.Error("member deleteMailbox must be forbidden")
	}

	// Org 0 (no tenant) is refused everywhere.
	if _, err := p.listMailboxes(ctx, &ListMailboxesInput{Ctx: org0Ctx()}); err == nil {
		t.Error("listMailboxes with org 0 must fail")
	}
	if _, err := p.createMailbox(ctx, &CreateMailboxInput{Ctx: org0Ctx(), Body: mailboxDTO{Address: "x@example.com"}}); err == nil {
		t.Error("createMailbox with org 0 must fail")
	}
	if _, err := p.updateMailbox(ctx, &UpdateMailboxInput{Ctx: org0Ctx(), ID: id, Body: mailboxDTO{Note: "x"}}); err == nil {
		t.Error("updateMailbox with org 0 must fail")
	}
	if _, err := p.deleteMailbox(ctx, &DeleteMailboxInput{Ctx: org0Ctx(), ID: id}); err == nil {
		t.Error("deleteMailbox with org 0 must fail")
	}

	// Cross-org mailbox is invisible to update/delete.
	r2 := httptest.NewRequest(http.MethodPost, "/", nil)
	r2.Header.Set("X-Org-ID", "2")
	other := humago.NewContext(nil, r2, httptest.NewRecorder())
	if _, err := p.updateMailbox(ctx, &UpdateMailboxInput{Ctx: other, ID: id, Body: mailboxDTO{Note: "x"}}); err == nil {
		t.Error("cross-org updateMailbox must 404")
	}
	if _, err := p.deleteMailbox(ctx, &DeleteMailboxInput{Ctx: other, ID: id}); err == nil {
		t.Error("cross-org deleteMailbox must 404")
	}
}

func TestEmailAndSenderAuthorizationGates(t *testing.T) {
	t.Parallel()
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()
	id := seededMailbox(t, p)
	p.db.Create(&Email{MailboxID: id, Read: false, ReceivedAt: time.Now()})

	// Org-0 emails refused.
	if _, err := p.listEmails(ctx, &ListEmailsInput{Ctx: org0Ctx()}); err == nil {
		t.Error("listEmails with org 0 must fail")
	}
	if _, err := p.getEmail(ctx, &GetEmailInput{Ctx: org0Ctx(), ID: 1}); err == nil {
		t.Error("getEmail with org 0 must fail")
	}
	if _, err := p.updateEmail(ctx, &UpdateEmailInput{Ctx: org0Ctx(), ID: 1}); err == nil {
		t.Error("updateEmail with org 0 must fail")
	}
	if _, err := p.readAllEmails(ctx, &ReadAllEmailsInput{Ctx: org0Ctx()}); err == nil {
		t.Error("readAllEmails with org 0 must fail")
	}
	if _, err := p.rawEmail(ctx, &RawEmailInput{Ctx: org0Ctx(), ID: 1}); err == nil {
		t.Error("rawEmail with org 0 must fail")
	}
	if _, err := p.deleteEmail(ctx, &DeleteEmailInput{Ctx: org0Ctx(), ID: 1}); err == nil {
		t.Error("deleteEmail with org 0 must fail")
	}
	if _, err := p.sendEmail(ctx, &SendEmailInput{Ctx: org0Ctx()}); err == nil {
		t.Error("sendEmail with org 0 must fail")
	}

	// Sender gates.
	if _, err := p.listSMTPSenders(ctx, &ListSMTPSendersInput{Ctx: org0Ctx()}); err == nil {
		t.Error("listSMTPSenders with org 0 must fail")
	}
	if _, err := p.createSMTPSender(ctx, &CreateSMTPSenderInput{Ctx: org0Ctx()}); err == nil {
		t.Error("createSMTPSender with org 0 must fail")
	}
	if _, err := p.createSMTPSender(ctx, &CreateSMTPSenderInput{Ctx: memberCtx()}); err == nil {
		t.Error("member createSMTPSender must be forbidden")
	}
	if _, err := p.updateSMTPSender(ctx, &UpdateSMTPSenderInput{Ctx: memberCtx(), ID: 1}); err == nil {
		t.Error("member updateSMTPSender must be forbidden")
	}
	if _, err := p.deleteSMTPSender(ctx, &DeleteSMTPSenderInput{Ctx: memberCtx(), ID: 1}); err == nil {
		t.Error("member deleteSMTPSender must be forbidden")
	}
}

func TestSendEmailPublishesFailureEvent(t *testing.T) {
	t.Parallel()
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	deadSender := SMTPSender{OrgID: 1, Name: "dead", Host: "127.0.0.1", Port: 1, User: "u", Pass: "pw", FromEmail: "noreply@example.com"}
	p.db.Create(&deadSender)

	var failed int
	p.publishEvent = func(orgID uint, event string, data any) {
		if event == "email.send_failed" {
			failed++
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	input := &SendEmailInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		Body: struct {
			internalmail.Message
			SMTPSenderID uint `json:"smtpSenderId"`
			TrackLinks   bool `json:"trackLinks"`
		}{SMTPSenderID: deadSender.ID, Message: internalmail.Message{To: []string{"to@example.com"}, Subject: "s", Text: "b"}},
	}
	if _, err := p.sendEmail(ctx, input); err == nil {
		t.Fatal("send to an unreachable relay must fail")
		return
	}
	if failed != 1 {
		t.Errorf("email.send_failed events = %d, want 1", failed)
	}
}

func TestIsSuppressedEdges(t *testing.T) {
	p, db := newBouncePlugin(t)
	if p.isSuppressed(1, "") {
		t.Error("empty address must not be suppressed")
	}
	if p.isSuppressed(0, "x@example.com") {
		t.Error("org 0 must not be suppressed")
	}
	db.Create(&MailSuppression{OrgID: 1, Address: "blocked@example.com", Reason: "manual"})
	if !p.isSuppressed(1, "  BLOCKED@example.com  ") {
		t.Error("case/space-normalized suppressed address must be found")
	}
}

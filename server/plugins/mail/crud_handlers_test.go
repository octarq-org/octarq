package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugins/dns"
)

func setupFullMailTestDB(t *testing.T) (*Plugin, func(req *http.Request) huma.Context) {
	t.Helper()
	db, p := setupMailboxTestDB(t)

	// Ensure p.encrypt / p.decrypt / p.requireRole are wired
	p.encrypt = func(b []byte) (string, error) { return "enc:" + string(b), nil }
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
	p.requireRole = func(r *http.Request, role string) bool {
		return r.Header.Get("X-Role") != "member"
	}

	// Create test domain for org 1
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    t.Name() + ".example.com",
		ForMail: true,
	})

	mkCtx := func(r *http.Request) huma.Context {
		if r.Header.Get("X-Org-ID") == "" {
			r.Header.Set("X-Org-ID", "1")
		}
		return humago.NewContext(nil, r, httptest.NewRecorder())
	}
	return p, mkCtx
}

func TestMailboxCRUDHandlers(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	// 1. Create Mailbox
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/mailboxes", nil)
	createIn := &CreateMailboxInput{
		Ctx:  mkCtx(reqCreate),
		Body: mailboxDTO{Address: "sales@" + t.Name() + ".example.com", Note: "Initial"},
	}
	outCreate, err := p.createMailbox(ctx, createIn)
	if err != nil {
		t.Fatalf("createMailbox error: %v", err)
	}
	mbID := outCreate.Body.ID

	// 2. List Mailboxes
	reqList := httptest.NewRequest(http.MethodGet, "/api/mailboxes", nil)
	listOut, err := p.listMailboxes(ctx, &ListMailboxesInput{Ctx: mkCtx(reqList)})
	if err != nil || len(listOut.Body) == 0 {
		t.Fatalf("listMailboxes error=%v, body len=%d", err, len(listOut.Body))
	}

	// 3. Update Mailbox
	reqUp := httptest.NewRequest(http.MethodPut, "/api/mailboxes/1", nil)
	upOut, err := p.updateMailbox(ctx, &UpdateMailboxInput{
		Ctx:  mkCtx(reqUp),
		ID:   mbID,
		Body: mailboxDTO{Note: "Updated note"},
	})
	if err != nil || upOut.Body.Note != "Updated note" {
		t.Fatalf("updateMailbox error=%v, out=%+v", err, upOut)
	}

	// 4. Delete Mailbox - forbidden for non-admin
	reqDelUser := httptest.NewRequest(http.MethodDelete, "/api/mailboxes/1", nil)
	reqDelUser.Header.Set("X-Role", "member")
	_, err = p.deleteMailbox(ctx, &DeleteMailboxInput{Ctx: mkCtx(reqDelUser), ID: mbID})
	if err == nil {
		t.Error("expected 403 when deleting mailbox as member")
	}

	// Delete Mailbox - success as admin
	reqDelAdmin := httptest.NewRequest(http.MethodDelete, "/api/mailboxes/1", nil)
	reqDelAdmin.Header.Set("X-Role", "admin")
	delOut, err := p.deleteMailbox(ctx, &DeleteMailboxInput{Ctx: mkCtx(reqDelAdmin), ID: mbID})
	if err != nil || !delOut.Body["ok"] {
		t.Fatalf("deleteMailbox error=%v", err)
	}
}

func TestEmailCRUDHandlers(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	// Seed mailbox & email
	mb := Mailbox{OrgID: 1, Address: "info@" + t.Name() + ".example.com", Enabled: true}
	p.db.Create(&mb)

	email := Email{
		MailboxID:  mb.ID,
		FromAddr:   "sender@test.com",
		ToAddr:     "info@" + t.Name() + ".example.com",
		Subject:    "Welcome to Octarq",
		Text:       "Hello text body",
		HTML:       "<p>Hello HTML body</p>",
		Raw:        []byte("From: sender@test.com\r\nTo: info@example.com\r\nSubject: Welcome\r\n\r\nHello"),
		Read:       false,
		ReceivedAt: time.Now(),
	}
	p.db.Create(&email)

	// 1. List Emails
	reqList := httptest.NewRequest(http.MethodGet, "/api/emails?q=Welcome", nil)
	listOut, err := p.listEmails(ctx, &ListEmailsInput{
		Ctx:   mkCtx(reqList),
		Q:     "Welcome",
		Limit: 10,
	})
	if err != nil || len(listOut.Body) != 1 {
		t.Fatalf("listEmails error=%v, count=%d", err, len(listOut.Body))
	}

	// 2. Get Email
	reqGet := httptest.NewRequest(http.MethodGet, "/api/emails/1", nil)
	getOut, err := p.getEmail(ctx, &GetEmailInput{Ctx: mkCtx(reqGet), ID: email.ID})
	if err != nil || getOut.Body.Subject != "Welcome to Octarq" {
		t.Fatalf("getEmail error=%v, out=%+v", err, getOut)
	}

	// 3. Update Email (mark read)
	bTrue := true
	reqUp := httptest.NewRequest(http.MethodPut, "/api/emails/1", nil)
	upIn := &UpdateEmailInput{
		Ctx: mkCtx(reqUp),
		ID:  email.ID,
	}
	upIn.Body.Read = &bTrue
	noteVal := "Flagged"
	upIn.Body.Note = &noteVal

	upOut, err := p.updateEmail(ctx, upIn)
	if err != nil || !upOut.Body.Read || upOut.Body.Note != "Flagged" {
		t.Fatalf("updateEmail error=%v, out=%+v", err, upOut)
	}

	// 4. Raw Email
	reqRaw := httptest.NewRequest(http.MethodGet, "/api/emails/1/raw", nil)
	_, err = p.rawEmail(ctx, &RawEmailInput{Ctx: mkCtx(reqRaw), ID: email.ID})
	if err != nil {
		t.Fatalf("rawEmail error=%v", err)
	}

	// 5. Read All Emails
	reqReadAll := httptest.NewRequest(http.MethodPost, "/api/emails/read-all", nil)
	readAllOut, err := p.readAllEmails(ctx, &ReadAllEmailsInput{
		Ctx: mkCtx(reqReadAll),
	})
	if err != nil || readAllOut.Body["ok"] != true {
		t.Fatalf("readAllEmails error=%v", err)
	}

	// 6. Delete Email
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/emails/1", nil)
	delOut, err := p.deleteEmail(ctx, &DeleteEmailInput{Ctx: mkCtx(reqDel), ID: email.ID})
	if err != nil || !delOut.Body["ok"] {
		t.Fatalf("deleteEmail error=%v", err)
	}
}

func TestSMTPSenderCRUDHandlers(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	// 1. Create SMTPSender
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/smtp-senders", nil)
	reqCreate.Header.Set("X-Role", "admin")
	createIn := &CreateSMTPSenderInput{
		Ctx: mkCtx(reqCreate),
	}
	createIn.Body.Name = "Main SMTP"
	createIn.Body.Host = "smtp.mailgun.org"
	createIn.Body.Port = 587
	createIn.Body.User = "postmaster@example.com"
	createIn.Body.Pass = "secretpass"
	createIn.Body.FromEmail = "noreply@example.com"

	outCreate, err := p.createSMTPSender(ctx, createIn)
	if err != nil {
		t.Fatalf("createSMTPSender error: %v", err)
	}
	smtpID := outCreate.Body.ID

	// 2. List SMTPSenders
	reqList := httptest.NewRequest(http.MethodGet, "/api/smtp-senders", nil)
	listOut, err := p.listSMTPSenders(ctx, &ListSMTPSendersInput{Ctx: mkCtx(reqList)})
	if err != nil || len(listOut.Body) != 1 {
		t.Fatalf("listSMTPSenders error=%v, count=%d", err, len(listOut.Body))
	}

	// 3. Update SMTPSender
	reqUp := httptest.NewRequest(http.MethodPut, "/api/smtp-senders/1", nil)
	reqUp.Header.Set("X-Role", "admin")
	upIn := &UpdateSMTPSenderInput{
		Ctx: mkCtx(reqUp),
		ID:  smtpID,
	}
	newName := "Updated Main SMTP"
	upIn.Body.Name = &newName
	upOut, err := p.updateSMTPSender(ctx, upIn)
	if err != nil || upOut.Body.Name != "Updated Main SMTP" {
		t.Fatalf("updateSMTPSender error=%v, out=%+v", err, upOut)
	}

	// 4. Delete SMTPSender
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/smtp-senders/1", nil)
	reqDel.Header.Set("X-Role", "admin")
	delOut, err := p.deleteSMTPSender(ctx, &DeleteSMTPSenderInput{Ctx: mkCtx(reqDel), ID: smtpID})
	if err != nil || !delOut.Body["ok"] {
		t.Fatalf("deleteSMTPSender error=%v", err)
	}
}

func TestMailOverviewHandler(t *testing.T) {
	t.Parallel()

	p, _ := setupFullMailTestDB(t)

	p.db.Create(&Mailbox{OrgID: 1, Address: "info@example.com", Enabled: true})
	p.db.Create(&Email{MailboxID: 1, Subject: "Test", Read: false, ReceivedAt: time.Now()})

	stats := p.overview(1, false)
	if stats["mailboxesCount"] == 0 && stats["mailboxes"] == 0 {
		t.Errorf("overview expected non-zero mailboxes count, got %+v", stats)
	}
}

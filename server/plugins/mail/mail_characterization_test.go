package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

// Stage 1: Characterization Locking Test for plugins/mail
// Locks exported interfaces, state machines, tenant isolation boundaries, and fail-closed error responses.

func TestMail_Characterization_InterfaceContracts(t *testing.T) {
	t.Parallel()
	var (
		_ plugin.Plugin       = (*Plugin)(nil)
		_ plugin.Describer    = (*Plugin)(nil)
		_ plugin.MenuProvider = (*Plugin)(nil)
		_ plugin.HelpDocsFS   = (*Plugin)(nil)
	)

	p := New()
	if p.Name() != "mail" {
		t.Fatalf("expected plugin Name 'mail', got %q", p.Name())
	}
	desc := p.Describe()
	if desc.Title != "Mail" {
		t.Fatalf("expected Describe().Title 'Mail', got %q", desc.Title)
	}
}

func TestMail_Characterization_SystemSenderFailClosed(t *testing.T) {
	t.Parallel()
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)

	// 1. With NO senders in DB, systemSender MUST fail closed with clear error.
	_, err := p.systemSender()
	if err == nil {
		t.Fatal("expected systemSender to fail closed when no senders exist in db")
	}

	// Seed senders in DB (Org 1 and Org 2)
	p.db.Create(&SMTPSender{ID: 1, OrgID: 1, Name: "tenant1-sender"})
	p.db.Create(&SMTPSender{ID: 2, OrgID: 2, Name: "tenant2-sender"})

	// 2. With no mail_system_sender_id configured, MUST fail closed (no lowest-ID fallback).
	_, err = p.systemSender()
	if err == nil {
		t.Fatal("expected systemSender to fail closed when mail_system_sender_id is not configured")
	}

	// 3. With stale/deleted sender ID, MUST fail closed (no lowest-ID fallback).
	settings := map[string]string{"mail_system_sender_id": "999"}
	p.getGlobalSetting = func(k string) string { return settings[k] }
	_, err = p.systemSender()
	if err == nil {
		t.Fatal("expected systemSender to fail closed when mail_system_sender_id points to non-existent sender")
	}

	// 4. With valid configured sender ID, systemSender resolves configured sender.
	settings["mail_system_sender_id"] = "2"
	s, err := p.systemSender()
	if err != nil {
		t.Fatalf("systemSender failed with valid sender: %v", err)
	}
	if s.ID != 2 || s.OrgID != 2 {
		t.Fatalf("resolved sender ID=%d OrgID=%d, expected ID=2 OrgID=2", s.ID, s.OrgID)
	}
}

func TestMail_Characterization_MultiTenantIsolation(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	// Seed mailboxes and emails in Org 1 and Org 2
	mb1 := Mailbox{OrgID: 1, Address: "org1@example.com", Enabled: true}
	mb2 := Mailbox{OrgID: 2, Address: "org2@example.com", Enabled: true}
	p.db.Create(&mb1)
	p.db.Create(&mb2)

	e1 := Email{MailboxID: mb1.ID, Subject: "Org1 Private", ReceivedAt: time.Now()}
	e2 := Email{MailboxID: mb2.ID, Subject: "Org2 Secret", ReceivedAt: time.Now()}
	p.db.Create(&e1)
	p.db.Create(&e2)

	// Org 1 queries listEmails
	req1 := httptest.NewRequest(http.MethodGet, "/api/emails", nil)
	req1.Header.Set("X-Org-ID", "1")
	out1, err := p.listEmails(ctx, &ListEmailsInput{Ctx: mkCtx(req1)})
	if err != nil {
		t.Fatalf("listEmails org 1: %v", err)
	}
	for _, em := range out1.Body {
		if em.Subject == "Org2 Secret" {
			t.Fatal("cross-tenant data bleed: Org 1 saw Org 2's email")
		}
	}

	// Org 1 attempts to get Org 2's email
	reqGet := httptest.NewRequest(http.MethodGet, "/api/emails/2", nil)
	reqGet.Header.Set("X-Org-ID", "1")
	_, errGet := p.getEmail(ctx, &GetEmailInput{Ctx: mkCtx(reqGet), ID: e2.ID})
	if errGet == nil {
		t.Fatal("expected 404 when Org 1 queries Org 2's email")
	}

	// Org 1 attempts to delete Org 2's email
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/emails/2", nil)
	reqDel.Header.Set("X-Org-ID", "1")
	_, errDel := p.deleteEmail(ctx, &DeleteEmailInput{Ctx: mkCtx(reqDel), ID: e2.ID})
	if errDel == nil {
		t.Fatal("expected error when Org 1 attempts to delete Org 2's email")
	}

	// Verify e2 is still intact in DB
	var checkE2 Email
	if err := p.db.Where("id = ?", e2.ID).First(&checkE2).Error; err != nil {
		t.Fatalf("e2 was deleted or not found: %v", err)
	}
}

func TestMail_Characterization_SuppressionFiltering(t *testing.T) {
	t.Parallel()
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)

	p.db.Create(&MailSuppression{OrgID: 1, Address: "bounced@example.com", Reason: "hard_bounce"})

	if !p.isSuppressed(1, "bounced@example.com") {
		t.Fatal("expected address to be suppressed for org 1")
	}
	if p.isSuppressed(2, "bounced@example.com") {
		t.Fatal("suppression in org 1 must not bleed into org 2")
	}
}

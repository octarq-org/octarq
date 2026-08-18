package mail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	dns "github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func setupCrossOrgMailDB(t *testing.T) (*Plugin, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	p := New()
	if err := db.AutoMigrate(append(append(models.AllModels(), p.Models()...), &dns.Domain{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	p.Mount(nil, &plugin.Context{
		DB: db,
		OrgID: func(r *http.Request) uint {
			if val := r.Header.Get("X-Org-ID"); val != "" {
				var id uint
				fmt.Sscanf(val, "%d", &id)
				return id
			}
			return 1
		},
		RequireRole: func(r *http.Request, _ string) bool {
			return r.Header.Get("X-Role") != "member"
		},
	})
	p.encrypt = func(b []byte) (string, error) { return string(b), nil }
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
	return p, db
}

func makeOrgCtx(orgID uint, role string) huma.Context {
	req := httptest.NewRequest(http.MethodGet, "/api/emails", nil)
	req.Header.Set("X-Org-ID", fmt.Sprintf("%d", orgID))
	req.Header.Set("X-Role", role)
	return humago.NewContext(nil, req, httptest.NewRecorder())
}

func TestMailCrossOrgIsolation(t *testing.T) {
	p, db := setupCrossOrgMailDB(t)
	ctx := context.Background()

	const org1 uint = 1
	const org2 uint = 2

	// Seed Org 1 domain & mailbox
	db.Create(&dns.Domain{OrgID: org1, Name: "org1.example.com", ForMail: true})
	mb1 := Mailbox{OrgID: org1, Address: "user1@org1.example.com", Note: "Org1 MB", Enabled: true}
	db.Create(&mb1)

	// Seed Org 2 domain & mailbox
	db.Create(&dns.Domain{OrgID: org2, Name: "org2.example.com", ForMail: true})
	mb2 := Mailbox{OrgID: org2, Address: "user2@org2.example.com", Note: "Org2 MB", Enabled: true}
	db.Create(&mb2)

	// Seed Org 1 emails
	e1_1 := Email{
		MailboxID:  mb1.ID,
		FromAddr:   "sender1@remote.com",
		Subject:    "Org 1 Email 1",
		Text:       "Hello from Org 1",
		Raw:        []byte("Subject: Org 1 Email 1\r\n\r\nHello from Org 1"),
		Read:       false,
		ReceivedAt: time.Now().Add(-10 * time.Minute),
	}
	e1_2 := Email{
		MailboxID:  mb1.ID,
		FromAddr:   "sender2@remote.com",
		Subject:    "Org 1 Email 2",
		Text:       "Another message for Org 1",
		Raw:        []byte("Subject: Org 1 Email 2\r\n\r\nAnother message for Org 1"),
		Read:       true,
		ReceivedAt: time.Now().Add(-5 * time.Minute),
	}
	db.Create(&e1_1)
	db.Create(&e1_2)

	// Seed Org 2 emails
	e2_1 := Email{
		MailboxID:  mb2.ID,
		FromAddr:   "confidential@partner.com",
		Subject:    "Confidential Org 2 Report",
		Text:       "Top secret financial data for Org 2",
		Note:       "org2 secret note",
		Raw:        []byte("Subject: Confidential Org 2 Report\r\n\r\nTop secret financial data for Org 2"),
		Read:       false,
		ReceivedAt: time.Now().Add(-8 * time.Minute),
	}
	e2_2 := Email{
		MailboxID:  mb2.ID,
		FromAddr:   "alerts@monitor.com",
		Subject:    "Org 2 Alert",
		Text:       "Alert for Org 2",
		Raw:        []byte("Subject: Org 2 Alert\r\n\r\nAlert for Org 2"),
		Read:       true,
		ReceivedAt: time.Now().Add(-2 * time.Minute),
	}
	db.Create(&e2_1)
	db.Create(&e2_2)

	ctxOrg1Admin := makeOrgCtx(org1, "admin")
	ctxOrg2Admin := makeOrgCtx(org2, "admin")

	// ==================== 1. Mailbox Endpoints Isolation ====================

	// 1a. List Mailboxes: Org 1 must only see its own mailbox
	mbListOut, err := p.listMailboxes(ctx, &ListMailboxesInput{Ctx: ctxOrg1Admin})
	if err != nil {
		t.Fatalf("listMailboxes: %v", err)
	}
	if len(mbListOut.Body) != 1 || mbListOut.Body[0].ID != mb1.ID {
		t.Errorf("listMailboxes: Org 1 saw mailboxes %+v, want only mb1 (%d)", mbListOut.Body, mb1.ID)
	}

	// 1b. Update Mailbox: Org 1 cannot update Org 2's mailbox
	updateNote := "malicious-update"
	_, err = p.updateMailbox(ctx, &UpdateMailboxInput{
		Ctx:  ctxOrg1Admin,
		ID:   mb2.ID,
		Body: mailboxDTO{Note: updateNote},
	})
	if err == nil {
		t.Errorf("updateMailbox: expected error when Org 1 updates Org 2 mailbox, got nil")
	}
	var mb2Reload Mailbox
	db.First(&mb2Reload, mb2.ID)
	if mb2Reload.Note == updateNote {
		t.Fatalf("updateMailbox: Org 2 mailbox was modified by Org 1")
	}

	// 1c. Delete Mailbox: Org 1 cannot delete Org 2's mailbox
	_, err = p.deleteMailbox(ctx, &DeleteMailboxInput{
		Ctx: ctxOrg1Admin,
		ID:  mb2.ID,
	})
	if err == nil {
		t.Errorf("deleteMailbox: expected error when Org 1 deletes Org 2 mailbox, got nil")
	}
	var count int64
	db.Model(&Mailbox{}).Where("id = ?", mb2.ID).Count(&count)
	if count == 0 {
		t.Fatalf("deleteMailbox: Org 2 mailbox was deleted by Org 1")
	}

	// ==================== 2. Email Endpoints Isolation (mailbox_id IN) ====================

	// 2a. listEmails (mail.go:218): Org 1 listing emails should never include Org 2 emails
	emailListOut, err := p.listEmails(ctx, &ListEmailsInput{Ctx: ctxOrg1Admin})
	if err != nil {
		t.Fatalf("listEmails: %v", err)
	}
	for _, em := range emailListOut.Body {
		if em.MailboxID != mb1.ID {
			t.Errorf("listEmails: Org 1 saw email belonging to mailbox %d, want only %d", em.MailboxID, mb1.ID)
		}
		if em.ID == e2_1.ID || em.ID == e2_2.ID {
			t.Errorf("listEmails: Org 1 saw Org 2 email %d (%s)", em.ID, em.Subject)
		}
	}

	// 2b. listEmails with Org 2 mailbox filter: must return empty
	emailListFilterOut, err := p.listEmails(ctx, &ListEmailsInput{
		Ctx:     ctxOrg1Admin,
		Mailbox: fmt.Sprintf("%d", mb2.ID),
	})
	if err != nil {
		t.Fatalf("listEmails with mailbox filter: %v", err)
	}
	if len(emailListFilterOut.Body) != 0 {
		t.Errorf("listEmails: Org 1 filtering by Org 2 mailbox got %d emails, want 0", len(emailListFilterOut.Body))
	}

	// 2c. listEmails with search query matching Org 2 email: must return empty
	emailSearchOut, err := p.listEmails(ctx, &ListEmailsInput{
		Ctx: ctxOrg1Admin,
		Q:   "Confidential",
	})
	if err != nil {
		t.Fatalf("listEmails search: %v", err)
	}
	if len(emailSearchOut.Body) != 0 {
		t.Errorf("listEmails: Org 1 search leaked Org 2 email: %+v", emailSearchOut.Body)
	}

	// 2d. getEmail (mail.go:264): Org 1 cannot get Org 2's email
	_, err = p.getEmail(ctx, &GetEmailInput{
		Ctx: ctxOrg1Admin,
		ID:  e2_1.ID,
	})
	if err == nil {
		t.Errorf("getEmail: expected 404 when Org 1 reads Org 2 email, got nil")
	}

	// Verify getEmail did not auto-mark e2_1 as read
	var e2_1Reload Email
	db.First(&e2_1Reload, e2_1.ID)
	if e2_1Reload.Read {
		t.Errorf("getEmail: Org 2 unread email was marked as read by Org 1 getEmail call")
	}

	// 2e. updateEmail (mail.go:302): Org 1 cannot update Org 2's email
	readVal := true
	tamperedNote := "tampered-by-org1"
	upIn := &UpdateEmailInput{
		Ctx: ctxOrg1Admin,
		ID:  e2_1.ID,
	}
	upIn.Body.Read = &readVal
	upIn.Body.Note = &tamperedNote
	_, err = p.updateEmail(ctx, upIn)
	if err == nil {
		t.Errorf("updateEmail: expected 404 when Org 1 updates Org 2 email, got nil")
	}
	db.First(&e2_1Reload, e2_1.ID)
	if e2_1Reload.Read || e2_1Reload.Note == tamperedNote {
		t.Fatalf("updateEmail: Org 2 email was mutated by Org 1 (read=%v, note=%q)", e2_1Reload.Read, e2_1Reload.Note)
	}

	// 2f. readAllEmails (mail.go:339): Org 1 calling readAllEmails must not affect Org 2
	_, err = p.readAllEmails(ctx, &ReadAllEmailsInput{Ctx: ctxOrg1Admin})
	if err != nil {
		t.Fatalf("readAllEmails: %v", err)
	}
	db.First(&e2_1Reload, e2_1.ID)
	if e2_1Reload.Read {
		t.Fatalf("readAllEmails: Org 1 readAllEmails marked Org 2 email as read")
	}

	// 2g. readAllEmails with Org 2 mailbox parameter: must not affect Org 2
	_, err = p.readAllEmails(ctx, &ReadAllEmailsInput{
		Ctx:     ctxOrg1Admin,
		Mailbox: fmt.Sprintf("%d", mb2.ID),
	})
	if err != nil {
		t.Fatalf("readAllEmails with mailbox: %v", err)
	}
	db.First(&e2_1Reload, e2_1.ID)
	if e2_1Reload.Read {
		t.Fatalf("readAllEmails with mailbox filter: Org 2 email marked read by Org 1")
	}

	// 2h. rawEmail (mail.go:371): Org 1 cannot download Org 2's raw email
	_, err = p.rawEmail(ctx, &RawEmailInput{
		Ctx: ctxOrg1Admin,
		ID:  e2_1.ID,
	})
	if err == nil {
		t.Errorf("rawEmail: expected 404 when Org 1 downloads Org 2 raw email, got nil")
	}

	// 2i. deleteEmail (mail.go:430 / 444): Org 1 cannot delete Org 2's email
	_, err = p.deleteEmail(ctx, &DeleteEmailInput{
		Ctx: ctxOrg1Admin,
		ID:  e2_1.ID,
	})
	if err == nil {
		t.Errorf("deleteEmail: expected 404 when Org 1 deletes Org 2 email, got nil")
	}
	db.Model(&Email{}).Where("id = ?", e2_1.ID).Count(&count)
	if count == 0 {
		t.Fatalf("deleteEmail: Org 2 email was deleted by Org 1")
	}

	// ==================== 3. Reverse Direction Isolation (Org 2 vs Org 1) ====================

	// Org 2 cannot read Org 1 email
	_, err = p.getEmail(ctx, &GetEmailInput{Ctx: ctxOrg2Admin, ID: e1_1.ID})
	if err == nil {
		t.Errorf("getEmail: expected error for Org 2 reading Org 1 email")
	}

	// Org 2 cannot update Org 1 email
	_, err = p.updateEmail(ctx, &UpdateEmailInput{Ctx: ctxOrg2Admin, ID: e1_1.ID})
	if err == nil {
		t.Errorf("updateEmail: expected error for Org 2 updating Org 1 email")
	}

	// Org 2 cannot delete Org 1 email
	_, err = p.deleteEmail(ctx, &DeleteEmailInput{Ctx: ctxOrg2Admin, ID: e1_1.ID})
	if err == nil {
		t.Errorf("deleteEmail: expected error for Org 2 deleting Org 1 email")
	}

	// Org 2 listEmails only returns Org 2 emails
	org2List, err := p.listEmails(ctx, &ListEmailsInput{Ctx: ctxOrg2Admin})
	if err != nil {
		t.Fatalf("listEmails for Org 2: %v", err)
	}
	if len(org2List.Body) != 2 {
		t.Errorf("listEmails for Org 2: got %d emails, want 2", len(org2List.Body))
	}
	for _, em := range org2List.Body {
		if em.MailboxID != mb2.ID {
			t.Errorf("listEmails for Org 2 saw non-Org2 mailbox %d", em.MailboxID)
		}
	}
}

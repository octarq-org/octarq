package mail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListMailboxesComputesUnread(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodGet, "/api/mailboxes", nil)
	mb := Mailbox{OrgID: 1, Address: "unread@example.com", Enabled: true}
	p.db.Create(&mb)
	p.db.Create(&Email{MailboxID: mb.ID, Read: false, Subject: "fresh", ReceivedAt: time.Now()})
	p.db.Create(&Email{MailboxID: mb.ID, Read: true, Subject: "read", ReceivedAt: time.Now()})

	out, err := p.listMailboxes(ctx, &ListMailboxesInput{Ctx: mkCtx(req)})
	if err != nil {
		t.Fatalf("listMailboxes: %v", err)
	}
	if len(out.Body) != 1 || out.Body[0].Unread != 1 {
		t.Errorf("unread count = %+v, want 1", out.Body)
	}
}

func TestReadAllEmailsScopesToOrg(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	mb1 := Mailbox{OrgID: 1, Address: "a@example.com", Enabled: true}
	mb2 := Mailbox{OrgID: 2, Address: "b@example.com", Enabled: true}
	p.db.Create(&mb1)
	p.db.Create(&mb2)
	p.db.Create(&Email{MailboxID: mb1.ID, Read: false, ReceivedAt: time.Now()})
	p.db.Create(&Email{MailboxID: mb1.ID, Read: false, ReceivedAt: time.Now()})
	p.db.Create(&Email{MailboxID: mb2.ID, Read: false, ReceivedAt: time.Now()})

	req := httptest.NewRequest(http.MethodPost, "/api/emails/read-all", nil)
	// Scope to org 1 mailbox 1.
	out, err := p.readAllEmails(ctx, &ReadAllEmailsInput{
		Ctx:     mkCtx(req),
		Mailbox: fmt.Sprint(mb1.ID),
	})
	if err != nil {
		t.Fatalf("readAllEmails: %v", err)
	}
	if out.Body["updated"].(int64) != 2 {
		t.Errorf("updated = %v, want 2 (org 1 only, org 2 untouched)", out.Body["updated"])
	}
}

func TestListEmailsMailboxAndPagingFilters(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "in@example.com", Enabled: true}
	p.db.Create(&mb)
	for i := 0; i < 3; i++ {
		p.db.Create(&Email{MailboxID: mb.ID, Subject: fmt.Sprintf("subj-%d", i), Text: fmt.Sprintf("hello %d", i), ReceivedAt: time.Now()})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/emails", nil)
	// Mailbox filter.
	out, err := p.listEmails(ctx, &ListEmailsInput{Ctx: mkCtx(req), Mailbox: fmt.Sprint(mb.ID), Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("listEmails: %v", err)
	}
	if len(out.Body) != 2 {
		t.Errorf("limit/offset = %d, want 2", len(out.Body))
	}
	// Q filter.
	outQ, err := p.listEmails(ctx, &ListEmailsInput{Ctx: mkCtx(req), Q: "hello 2"})
	if err != nil {
		t.Fatalf("listEmails q: %v", err)
	}
	if len(outQ.Body) != 1 || !strings.Contains(outQ.Body[0].Subject, "2") {
		t.Errorf("q filter: %+v", outQ.Body)
	}
}

func TestGetEmailMarksUnreadAsRead(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "r@example.com", Enabled: true}
	p.db.Create(&mb)
	em := Email{MailboxID: mb.ID, Read: false, Subject: "s", ReceivedAt: time.Now()}
	p.db.Create(&em)

	req := httptest.NewRequest(http.MethodGet, "/api/emails/1", nil)
	out, err := p.getEmail(ctx, &GetEmailInput{Ctx: mkCtx(req), ID: em.ID})
	if err != nil {
		t.Fatalf("getEmail: %v", err)
	}
	if !out.Body.Read {
		t.Error("getEmail must mark email as read in response")
	}
	var reloaded Email
	p.db.First(&reloaded, em.ID)
	if !reloaded.Read {
		t.Error("getEmail did not persist read=true")
	}
}

func TestDeleteEmailRemovesBlobsAndRow(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "d@example.com", Enabled: true}
	p.db.Create(&mb)
	em := Email{MailboxID: mb.ID, StorageKey: "mail/1/custom.eml", ReceivedAt: time.Now()}
	p.db.Create(&em)

	dbProv := NewDBStorageProvider(p.db)
	if err := dbProv.Put(ctx, "mail/1/custom.eml", []byte("raw data")); err != nil {
		t.Fatalf("put raw: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/emails/1", nil)
	if _, err := p.deleteEmail(ctx, &DeleteEmailInput{Ctx: mkCtx(req), ID: em.ID}); err != nil {
		t.Fatalf("deleteEmail: %v", err)
	}

	var blobs int64
	p.db.Model(&MailRawBlob{}).Count(&blobs)
	if blobs != 0 {
		t.Errorf("db blob after delete = %d, want 0", blobs)
	}
	var rows int64
	p.db.Model(&Email{}).Count(&rows)
	if rows != 0 {
		t.Errorf("email rows after delete = %d, want 0", rows)
	}
}

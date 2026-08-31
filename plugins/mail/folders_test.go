package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestContactsUpsertAndSearchIsolation(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	// Org 1 contacts
	p.upsertContact(1, "Alice Smith <alice@example.com>")
	p.upsertContact(1, "alice@example.com") // increments count
	p.upsertContact(1, "Bob Jones <bob@example.com>")

	// Org 2 contact
	p.upsertContact(2, "Charlie Brown <charlie@other.com>")

	asOrg := func(org string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/mail/contacts", nil)
		req.Header.Set("X-Org-ID", org)
		return req
	}

	// Org 1 lists contacts
	out1, err := p.listContacts(ctx, &ListContactsInput{Ctx: mkCtx(asOrg("1"))})
	if err != nil {
		t.Fatalf("listContacts org 1 failed: %v", err)
	}
	if len(out1.Body) != 2 {
		t.Fatalf("org 1 contacts count = %d, want 2", len(out1.Body))
	}
	// Alice had 2 interactions, should be first
	if out1.Body[0].Address != "alice@example.com" || out1.Body[0].InteractionCount != 2 {
		t.Fatalf("expected alice first with count 2, got %+v", out1.Body[0])
	}
	if out1.Body[0].Name != "Alice Smith" {
		t.Fatalf("expected alice name 'Alice Smith', got %q", out1.Body[0].Name)
	}

	// Org 1 search query
	outSearch, err := p.listContacts(ctx, &ListContactsInput{Ctx: mkCtx(asOrg("1")), Q: "bob"})
	if err != nil || len(outSearch.Body) != 1 || outSearch.Body[0].Address != "bob@example.com" {
		t.Fatalf("search 'bob' failed: err=%v out=%+v", err, outSearch)
	}

	// Org 2 lists contacts (isolated)
	out2, err := p.listContacts(ctx, &ListContactsInput{Ctx: mkCtx(asOrg("2"))})
	if err != nil {
		t.Fatalf("listContacts org 2 failed: %v", err)
	}
	if len(out2.Body) != 1 || out2.Body[0].Address != "charlie@other.com" {
		t.Fatalf("org 2 contacts unexpected: %+v", out2.Body)
	}
}

func TestFoldersFilterAndSentRecording(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "team@example.com", Enabled: true}
	p.db.Create(&mb)

	eInbox := Email{MailboxID: mb.ID, Folder: "inbox", Subject: "In 1", ReceivedAt: time.Now()}
	eSent := Email{MailboxID: mb.ID, Folder: "sent", Subject: "Sent 1", ReceivedAt: time.Now()}
	eTrash := Email{MailboxID: mb.ID, Folder: "trash", Subject: "Trash 1", ReceivedAt: time.Now()}
	p.db.Create(&eInbox)
	p.db.Create(&eSent)
	p.db.Create(&eTrash)

	asOrg1 := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/emails", nil)
		req.Header.Set("X-Org-ID", "1")
		return req
	}

	// Filter inbox
	inboxRes, err := p.listEmails(ctx, &ListEmailsInput{Ctx: mkCtx(asOrg1()), Folder: "inbox"})
	if err != nil || len(inboxRes.Body) != 1 || inboxRes.Body[0].Subject != "In 1" {
		t.Fatalf("filter folder=inbox failed: err=%v res=%+v", err, inboxRes)
	}

	// Filter sent
	sentRes, err := p.listEmails(ctx, &ListEmailsInput{Ctx: mkCtx(asOrg1()), Folder: "sent"})
	if err != nil || len(sentRes.Body) != 1 || sentRes.Body[0].Subject != "Sent 1" {
		t.Fatalf("filter folder=sent failed: err=%v res=%+v", err, sentRes)
	}
}

func TestDraftSaveUpdateDelete(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "sender@org1.com", Enabled: true}
	p.db.Create(&mb)

	asOrg := func(org string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/mail/drafts", nil)
		req.Header.Set("X-Org-ID", org)
		return req
	}

	// 1. Create draft
	saveIn := &SaveDraftInput{Ctx: mkCtx(asOrg("1"))}
	saveIn.Body.MailboxID = mb.ID
	saveIn.Body.To = "client@example.com"
	saveIn.Body.Subject = "Draft v1"
	saveIn.Body.Text = "Initial draft text"

	saveOut, err := p.saveDraft(ctx, saveIn)
	if err != nil || saveOut.Body.ID == 0 {
		t.Fatalf("saveDraft failed: err=%v out=%+v", err, saveOut)
	}
	draftID := saveOut.Body.ID

	// Verify saved with folder = "drafts"
	var draft Email
	if err := p.db.First(&draft, draftID).Error; err != nil || draft.Folder != "drafts" {
		t.Fatalf("draft not saved properly: err=%v draft=%+v", err, draft)
	}

	// 2. Update draft
	updateIn := &SaveDraftInput{Ctx: mkCtx(asOrg("1"))}
	updateIn.Body.ID = draftID
	updateIn.Body.To = "client@example.com"
	updateIn.Body.Subject = "Draft v2"
	updateIn.Body.Text = "Updated draft text"
	updateOut, err := p.saveDraft(ctx, updateIn)
	if err != nil || updateOut.Body.Subject != "Draft v2" {
		t.Fatalf("update draft failed: err=%v out=%+v", err, updateOut)
	}

	// 3. Cross-org delete fails
	delInCross := &DeleteDraftInput{Ctx: mkCtx(asOrg("2")), ID: draftID}
	_, err = p.deleteDraft(ctx, delInCross)
	if err == nil {
		t.Fatalf("expected cross-org deleteDraft to fail, got nil")
	}

	// 4. Owner delete draft
	delIn := &DeleteDraftInput{Ctx: mkCtx(asOrg("1")), ID: draftID}
	delOut, err := p.deleteDraft(ctx, delIn)
	if err != nil || !delOut.Body["ok"] {
		t.Fatalf("deleteDraft failed: err=%v out=%+v", err, delOut)
	}

	var deleted Email
	if err := p.db.First(&deleted, draftID).Error; err == nil {
		t.Fatalf("draft should have been deleted")
	}
}

func TestUpdateEmailFolder(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "folder@org1.com", Enabled: true}
	p.db.Create(&mb)

	email := Email{MailboxID: mb.ID, Folder: "inbox", Subject: "Move test"}
	p.db.Create(&email)

	asOrg := func(org string) *http.Request {
		req := httptest.NewRequest(http.MethodPut, "/api/mail/emails/1/folder", nil)
		req.Header.Set("X-Org-ID", org)
		return req
	}

	// Move to trash
	upIn := &UpdateEmailFolderInput{Ctx: mkCtx(asOrg("1")), ID: email.ID}
	upIn.Body.Folder = "trash"
	upOut, err := p.updateEmailFolder(ctx, upIn)
	if err != nil || upOut.Body.Folder != "trash" {
		t.Fatalf("updateEmailFolder to trash failed: err=%v out=%+v", err, upOut)
	}

	// Invalid folder fails
	upInBad := &UpdateEmailFolderInput{Ctx: mkCtx(asOrg("1")), ID: email.ID}
	upInBad.Body.Folder = "invalid_folder_name"
	_, err = p.updateEmailFolder(ctx, upInBad)
	if err == nil {
		t.Fatalf("expected invalid folder update to fail")
	}

	// Cross-org update fails
	upInCross := &UpdateEmailFolderInput{Ctx: mkCtx(asOrg("2")), ID: email.ID}
	upInCross.Body.Folder = "spam"
	_, err = p.updateEmailFolder(ctx, upInCross)
	if err == nil {
		t.Fatalf("expected cross-org folder update to fail")
	}
}

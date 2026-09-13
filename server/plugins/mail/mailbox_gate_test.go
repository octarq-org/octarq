package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

func TestMailboxRoleGate_CreateMailbox_ForbiddenForMember(t *testing.T) {
	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	// createMailbox lowercases addresses, so use lower-cased name for comparisons
	lowerName := strings.ToLower(t.Name())

	// Member attempts to create a mailbox with valid input
	req := httptest.NewRequest(http.MethodPost, "/api/mailboxes", nil)
	req.Header.Set("X-Role", "member")
	in := &CreateMailboxInput{
		Ctx:  mkCtx(req),
		Body: mailboxDTO{Address: "member-created@" + lowerName + ".example.com", Note: "Member Note"},
	}

	_, err := p.createMailbox(ctx, in)
	if err == nil {
		t.Fatal("expected 403 error for member createMailbox, got nil")
		return
	}
	stErr, ok := err.(huma.StatusError)
	if !ok || stErr.GetStatus() != http.StatusForbidden {
		t.Fatalf("expected 403 StatusForbidden, got %v", err)
	}

	// Verify no DB write occurred
	var count int64
	p.db.Model(&Mailbox{}).Where("address = ?", "member-created@"+lowerName+".example.com").Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 mailbox in DB, found %d (member write bypassed gate!)", count)
	}

	// Member attempts to create mailbox with invalid input (no @) -> must still fail at role gate (403, not 400)
	reqBad := httptest.NewRequest(http.MethodPost, "/api/mailboxes", nil)
	reqBad.Header.Set("X-Role", "member")
	inBad := &CreateMailboxInput{
		Ctx:  mkCtx(reqBad),
		Body: mailboxDTO{Address: "invalid-no-at-domain"},
	}
	_, errBad := p.createMailbox(ctx, inBad)
	if errBad == nil {
		t.Fatal("expected 403 error for member createMailbox with bad input, got nil")
		return
	}
	stErrBad, okBad := errBad.(huma.StatusError)
	if !okBad || stErrBad.GetStatus() != http.StatusForbidden {
		t.Fatalf("expected 403 StatusForbidden before param validation, got status %v (err: %v)", stErrBad.GetStatus(), errBad)
	}
}

func TestMailboxRoleGate_CreateMailbox_AllowedForAdmin(t *testing.T) {
	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	// createMailbox lowercases the address; match that here
	lowerName := strings.ToLower(t.Name())

	req := httptest.NewRequest(http.MethodPost, "/api/mailboxes", nil)
	req.Header.Set("X-Role", "admin")
	in := &CreateMailboxInput{
		Ctx:  mkCtx(req),
		Body: mailboxDTO{Address: "admin-created@" + lowerName + ".example.com", Note: "Admin Note"},
	}

	out, err := p.createMailbox(ctx, in)
	if err != nil {
		t.Fatalf("expected admin createMailbox success, got error: %v", err)
	}
	if out.Body.Address != "admin-created@"+lowerName+".example.com" {
		t.Fatalf("unexpected created mailbox address: %s", out.Body.Address)
	}
}

func TestMailboxRoleGate_UpdateMailbox_ForbiddenForMember(t *testing.T) {
	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	// Seed existing mailbox
	mb := Mailbox{OrgID: 1, Address: "existing@" + t.Name() + ".example.com", Note: "Original Note", Enabled: true}
	if err := p.db.Create(&mb).Error; err != nil {
		t.Fatalf("seed mailbox error: %v", err)
	}

	enabledFalse := false
	req := httptest.NewRequest(http.MethodPut, "/api/mailboxes/1", nil)
	req.Header.Set("X-Role", "member")
	in := &UpdateMailboxInput{
		Ctx:  mkCtx(req),
		ID:   mb.ID,
		Body: mailboxDTO{Note: "Member Changed Note", Enabled: &enabledFalse},
	}

	_, err := p.updateMailbox(ctx, in)
	if err == nil {
		t.Fatal("expected 403 error for member updateMailbox, got nil")
		return
	}
	stErr, ok := err.(huma.StatusError)
	if !ok || stErr.GetStatus() != http.StatusForbidden {
		t.Fatalf("expected 403 StatusForbidden, got %v", err)
	}

	// Verify DB state was NOT modified
	var fresh Mailbox
	p.db.First(&fresh, mb.ID)
	if fresh.Note != "Original Note" || !fresh.Enabled {
		t.Fatalf("mailbox in DB was modified by member! Note=%q, Enabled=%v", fresh.Note, fresh.Enabled)
	}
}

func TestMailboxRoleGate_UpdateMailbox_AllowedForAdmin(t *testing.T) {
	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "existing-admin@" + t.Name() + ".example.com", Note: "Original Note", Enabled: true}
	p.db.Create(&mb)

	req := httptest.NewRequest(http.MethodPut, "/api/mailboxes/1", nil)
	req.Header.Set("X-Role", "admin")
	in := &UpdateMailboxInput{
		Ctx:  mkCtx(req),
		ID:   mb.ID,
		Body: mailboxDTO{Note: "Admin Updated Note"},
	}

	out, err := p.updateMailbox(ctx, in)
	if err != nil {
		t.Fatalf("expected admin updateMailbox success, got error: %v", err)
	}
	if out.Body.Note != "Admin Updated Note" {
		t.Fatalf("expected note to be updated, got %q", out.Body.Note)
	}
}

func TestMailboxRoleGate_NonGatedEndpoint_AllowedForMember(t *testing.T) {
	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	// Seed mailbox & email
	mb := Mailbox{OrgID: 1, Address: "inbox@" + t.Name() + ".example.com", Enabled: true}
	p.db.Create(&mb)

	email := Email{
		MailboxID:  mb.ID,
		FromAddr:   "sender@test.com",
		ToAddr:     "inbox@" + t.Name() + ".example.com",
		Subject:    "Test Email",
		Read:       false,
		ReceivedAt: time.Now(),
	}
	p.db.Create(&email)

	// Member updates email read state (non-gated operation)
	req := httptest.NewRequest(http.MethodPut, "/api/emails/1", nil)
	req.Header.Set("X-Role", "member")
	bTrue := true
	upIn := &UpdateEmailInput{
		Ctx: mkCtx(req),
		ID:  email.ID,
	}
	upIn.Body.Read = &bTrue

	out, err := p.updateEmail(ctx, upIn)
	if err != nil {
		t.Fatalf("expected member updateEmail to succeed, got error: %v", err)
	}
	if !out.Body.Read {
		t.Fatalf("expected email read state to be true")
	}
}

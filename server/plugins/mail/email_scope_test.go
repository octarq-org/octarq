package mail

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// Cross-org requests against another workspace's email id must fail with 404
// and leave the row untouched. The ownership constraint lives in the query
// itself (mailbox_id IN (SELECT id FROM mailboxes WHERE owner_id = ?)), so a
// future handler can't accidentally drop the check the way a programmatic
// post-hoc lookup could.
func TestEmailHandlersRejectOtherOrg(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "scoped@" + t.Name() + ".example.com", Enabled: true}
	p.db.Create(&mb)
	email := Email{
		MailboxID:  mb.ID,
		Subject:    "Org 1 only",
		Text:       "body",
		Raw:        []byte("Subject: Org 1 only\r\n\r\nbody"),
		Read:       false,
		ReceivedAt: time.Now(),
	}
	p.db.Create(&email)

	// Caller acting as org 2 (mkCtx keeps the X-Org-ID we set).
	asOrg := func(org string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/emails", nil)
		req.Header.Set("X-Org-ID", org)
		return req
	}

	// getEmail -> 404
	_, err := p.getEmail(ctx, &GetEmailInput{Ctx: mkCtx(asOrg("2")), ID: email.ID})
	assert404(t, err, "getEmail")

	// updateEmail -> 404, no mutation
	read := true
	note := "from-org-2"
	upIn := &UpdateEmailInput{Ctx: mkCtx(asOrg("2")), ID: email.ID}
	upIn.Body.Read = &read
	upIn.Body.Note = &note
	_, err = p.updateEmail(ctx, upIn)
	assert404(t, err, "updateEmail")

	// rawEmail -> 404
	_, err = p.rawEmail(ctx, &RawEmailInput{Ctx: mkCtx(asOrg("2")), ID: email.ID})
	assert404(t, err, "rawEmail")

	// deleteEmail -> 404 (the caller is an admin of org 2, so it isn't the
	// role gate rejecting the request — the query scope is)
	delReq := asOrg("2")
	delReq.Header.Set("X-Role", "admin")
	_, err = p.deleteEmail(ctx, &DeleteEmailInput{Ctx: mkCtx(delReq), ID: email.ID})
	assert404(t, err, "deleteEmail")

	// The email still exists and was never mutated.
	var reloaded Email
	if err := p.db.First(&reloaded, email.ID).Error; err != nil {
		t.Fatalf("email vanished after cross-org attempts: %v", err)
	}
	if reloaded.Read || reloaded.Note == note {
		t.Fatalf("cross-org request mutated the email: read=%v note=%q", reloaded.Read, reloaded.Note)
	}

	// The owning org is unaffected — the 404s above came from scoping, not a
	// broken query.
	getOut, err := p.getEmail(ctx, &GetEmailInput{Ctx: mkCtx(asOrg("1")), ID: email.ID})
	if err != nil || getOut.Body.Subject != "Org 1 only" {
		t.Fatalf("owner getEmail failed after cross-org attempts: err=%v out=%+v", err, getOut)
	}
}

func assert404(t *testing.T, err error, op string) {
	t.Helper()
	var se huma.StatusError
	if err == nil || !errors.As(err, &se) || se.GetStatus() != http.StatusNotFound {
		t.Fatalf("%s: expected 404 for another org's email, got %v", op, err)
	}
}

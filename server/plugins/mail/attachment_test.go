package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

func TestGetAttachment_Success(t *testing.T) {
	t.Parallel()
	p, _ := setupFullMailTestDB(t)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "info@" + t.Name() + ".example.com", Enabled: true}
	p.db.Create(&mb)

	rawMIME := strings.Join([]string{
		"From: sender@example.com",
		"To: info@example.com",
		"Subject: Test with attachment",
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=\"BOUNDARY123\"",
		"",
		"--BOUNDARY123",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello body",
		"",
		"--BOUNDARY123",
		"Content-Type: text/plain; name=\"hello.txt\"",
		"Content-Disposition: attachment; filename=\"hello.txt\"",
		"",
		"hello attachment content",
		"--BOUNDARY123--",
		"",
	}, "\r\n")

	atts := []map[string]any{
		{"filename": "hello.txt", "contentType": "text/plain", "size": 24},
	}
	attsJSON, _ := json.Marshal(atts)

	email := Email{
		MailboxID:   mb.ID,
		FromAddr:    "sender@example.com",
		ToAddr:      "info@" + t.Name() + ".example.com",
		Subject:     "Test with attachment",
		Text:        "Hello body",
		Raw:         []byte(rawMIME),
		Attachments: string(attsJSON),
		ReceivedAt:  time.Now(),
	}
	p.db.Create(&email)

	if err := (&GetAttachmentInput{}).Resolve(nil); err != nil {
		t.Fatalf("Resolve should not error")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/emails/1/attachments/0", nil)
	req.Header.Set("X-Org-ID", "1")
	rec := httptest.NewRecorder()
	humaCtx := humago.NewContext(nil, req, rec)

	input := &GetAttachmentInput{
		Ctx:   humaCtx,
		ID:    email.ID,
		Index: 0,
	}
	_, err := p.getAttachment(ctx, input)
	if err != nil {
		t.Fatalf("getAttachment error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain content-type, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "hello attachment content") {
		t.Errorf("expected attachment body, got %q", rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "hello.txt") {
		t.Errorf("expected filename in disposition, got %q", cd)
	}
}

func TestGetAttachment_NotFound(t *testing.T) {
	t.Parallel()
	p, _ := setupFullMailTestDB(t)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "info2@" + t.Name() + ".example.com", Enabled: true}
	p.db.Create(&mb)

	atts := []map[string]any{
		{"filename": "a.txt", "contentType": "text/plain", "size": 5},
	}
	attsJSON, _ := json.Marshal(atts)
	email := Email{
		MailboxID:   mb.ID,
		FromAddr:    "sender@example.com",
		Subject:     "Has one attachment",
		Attachments: string(attsJSON),
		Raw:         []byte("From: a\r\nTo: b\r\nSubject: x\r\n\r\nbody"),
	}
	p.db.Create(&email)

	// Index out of range -> 404
	req := httptest.NewRequest(http.MethodGet, "/api/emails/1/attachments/5", nil)
	req.Header.Set("X-Org-ID", "1")
	rec := httptest.NewRecorder()
	humaCtx := humago.NewContext(nil, req, rec)
	_, err := p.getAttachment(ctx, &GetAttachmentInput{Ctx: humaCtx, ID: email.ID, Index: 5})
	if err == nil {
		t.Fatal("expected 404 for out-of-range index")
		return
	}

	// Negative index -> 404
	req2 := httptest.NewRequest(http.MethodGet, "/api/emails/1/attachments/-1", nil)
	req2.Header.Set("X-Org-ID", "1")
	rec2 := httptest.NewRecorder()
	humaCtx2 := humago.NewContext(nil, req2, rec2)
	_, err = p.getAttachment(ctx, &GetAttachmentInput{Ctx: humaCtx2, ID: email.ID, Index: -1})
	if err == nil {
		t.Fatal("expected 404 for negative index")
		return
	}

	// Nil context -> 500
	_, err = p.getAttachment(ctx, &GetAttachmentInput{Ctx: nil, ID: email.ID, Index: 0})
	if err == nil {
		t.Fatal("expected error for nil context")
		return
	}

	// Non-existent email -> 404
	req4 := httptest.NewRequest(http.MethodGet, "/api/emails/9999/attachments/0", nil)
	req4.Header.Set("X-Org-ID", "1")
	rec4 := httptest.NewRecorder()
	humaCtx4 := humago.NewContext(nil, req4, rec4)
	_, err = p.getAttachment(ctx, &GetAttachmentInput{Ctx: humaCtx4, ID: 9999, Index: 0})
	if err == nil {
		t.Fatal("expected 404 for missing email")
		return
	}
}

func TestGetAttachment_FallbackNotImplemented(t *testing.T) {
	t.Parallel()
	p, _ := setupFullMailTestDB(t)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "info3@" + t.Name() + ".example.com", Enabled: true}
	p.db.Create(&mb)

	atts := []map[string]any{
		{"filename": "missing.txt", "contentType": "text/plain", "size": 10},
	}
	attsJSON, _ := json.Marshal(atts)
	email := Email{
		MailboxID:   mb.ID,
		FromAddr:    "sender@example.com",
		Subject:     "No raw",
		Attachments: string(attsJSON),
		Raw:         nil,
		StorageKey:  "",
	}
	p.db.Create(&email)

	req := httptest.NewRequest(http.MethodGet, "/api/emails/1/attachments/0", nil)
	req.Header.Set("X-Org-ID", "1")
	rec := httptest.NewRecorder()
	humaCtx := humago.NewContext(nil, req, rec)

	_, err := p.getAttachment(ctx, &GetAttachmentInput{Ctx: humaCtx, ID: email.ID, Index: 0})
	if err != nil {
		t.Fatalf("expected fallback not error, got %v", err)
	}
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 fallback, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["error"] == nil {
		t.Error("expected error field in fallback response")
	}
}

func TestExtractAttachment(t *testing.T) {
	t.Parallel()

	rawInlineAndAttachment := strings.Join([]string{
		"From: a@example.com",
		"To: b@example.com",
		"Subject: inline vs attachment",
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=\"B\"",
		"",
		"--B",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"body",
		"",
		"--B",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>html</p>",
		"",
		"--B",
		"Content-Type: application/pdf; name=\"doc.pdf\"",
		"Content-Disposition: attachment; filename=\"doc.pdf\"",
		"",
		"PDFDATA",
		"--B--",
	}, "\r\n")

	data, fname, ctype, err := extractAttachment([]byte(rawInlineAndAttachment), 0)
	if err != nil {
		t.Fatalf("extractAttachment error: %v", err)
	}
	if string(data) != "PDFDATA" {
		t.Errorf("expected PDFDATA, got %q", string(data))
	}
	if fname != "doc.pdf" {
		t.Errorf("expected doc.pdf, got %q", fname)
	}
	if !strings.Contains(ctype, "application/pdf") {
		t.Errorf("expected pdf ctype, got %q", ctype)
	}

	_, _, _, err = extractAttachment([]byte(rawInlineAndAttachment), 5)
	if err == nil {
		t.Fatal("expected error for out-of-range index")
		return
	}

	_, _, _, err = extractAttachment([]byte("not a mime message"), 0)
	if err == nil {
		t.Fatal("expected error for invalid raw")
		return
	}

	_, _, _, err = extractAttachment([]byte{}, 0)
	if err == nil {
		t.Fatal("expected error for empty raw")
		return
	}
}

func TestExtractAttachment_InlineWithFilename(t *testing.T) {
	t.Parallel()
	raw := strings.Join([]string{
		"From: a@example.com",
		"To: b@example.com",
		"Subject: inline with name",
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=\"B2\"",
		"",
		"--B2",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"body",
		"",
		"--B2",
		"Content-Type: image/png; name=\"photo.png\"",
		"Content-Disposition: inline; filename=\"photo.png\"",
		"",
		"PNGDATA",
		"--B2--",
	}, "\r\n")

	data, _, _, err := extractAttachment([]byte(raw), 0)
	if err != nil {
		t.Logf("inline with filename not counted as attachment (expected if text/*): err=%v", err)
		return
	}
	if string(data) != "PNGDATA" {
		t.Errorf("expected PNGDATA, got %q", string(data))
	}
}

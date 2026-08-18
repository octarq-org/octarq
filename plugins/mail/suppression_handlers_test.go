package mail

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

func TestSuppressionCRUDHandlers(t *testing.T) {
	t.Parallel()
	p, db := newBouncePlugin(t)
	db.Create(&models.Org{ID: 1, Slug: "acme", InboundToken: "tok"})

	audit := 0
	p.audit = func(*http.Request, string, string, uint, map[string]any) { audit++ }

	ctx := context.Background()

	// Empty list before any suppression.
	out, err := p.listSuppressions(ctx, &ListSuppressionsInput{Ctx: mkCtx2()})
	if err != nil {
		t.Fatalf("listSuppressions: %v", err)
	}
	if len(out.Body) != 0 {
		t.Fatalf("expected empty list, got %+v", out.Body)
	}

	// Create with a valid address.
	created, err := p.createSuppression(ctx, &CreateSuppressionInput{Ctx: mkCtx2(), Body: suppressionDTO{Address: "spam@example.com"}})
	if err != nil {
		t.Fatalf("createSuppression: %v", err)
	}
	if created.Body.Address != "spam@example.com" || created.Body.Reason != "manual" {
		t.Errorf("unexpected suppression: %+v", created.Body)
	}

	// Invalid address refused.
	if _, err := p.createSuppression(ctx, &CreateSuppressionInput{Ctx: mkCtx2(), Body: suppressionDTO{Address: "nope"}}); err == nil {
		t.Error("address without @ must be refused")
	}

	// Upsert: creating the same address again keeps a single row.
	if _, err := p.createSuppression(ctx, &CreateSuppressionInput{Ctx: mkCtx2(), Body: suppressionDTO{Address: "SPAM@example.com"}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var n int64
	db.Model(&MailSuppression{}).Where("owner_id = ? AND address = ?", 1, "spam@example.com").Count(&n)
	if n != 1 {
		t.Errorf("upsert produced %d rows, want 1", n)
	}

	// List reflects the row.
	out2, err := p.listSuppressions(ctx, &ListSuppressionsInput{Ctx: mkCtx2()})
	if err != nil {
		t.Fatalf("second list: %v", err)
	}
	if len(out2.Body) != 1 || out2.Body[0].Address != "spam@example.com" {
		t.Errorf("list after create: %+v", out2.Body)
	}

	// Delete missing -> 404.
	if _, err := p.deleteSuppression(ctx, &DeleteSuppressionInput{Ctx: mkCtx2(), ID: 999}); err == nil {
		t.Error("deleting a missing suppression must 404")
	}

	// Delete existing -> ok.
	delOut, err := p.deleteSuppression(ctx, &DeleteSuppressionInput{Ctx: mkCtx2(), ID: created.Body.ID})
	if err != nil || !delOut.Body["ok"] {
		t.Fatalf("deleteSuppression: %v", err)
	}
	if audit != 3 {
		t.Errorf("audit calls = %d, want 3 (create + upsert + delete)", audit)
	}
}

func mkCtx2() huma.Context {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	return humago.NewContext(nil, req, httptest.NewRecorder())
}

func TestInboundHandler(t *testing.T) {
	t.Parallel()
	p, db := newBouncePlugin(t)
	db.Create(&models.Org{ID: 1, Slug: "acme", InboundToken: "tok"})
	mb := Mailbox{OrgID: 1, Address: "bob@acme.example", Enabled: true}
	db.Create(&mb)

	raw := "From: a@x.com\r\nTo: bob@acme.example\r\nSubject: hi\r\n\r\nHello"

	ctx := context.Background()

	// Unknown org -> 404.
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/nope/email/inbound/tok", bytes.NewReader([]byte(raw)))
	if _, err := p.inbound(ctx, &InboundInput{Ctx: humago.NewContext(nil, req, httptest.NewRecorder()), OrgSlug: "nope", Token: "tok"}); err == nil {
		t.Error("unknown org must 404")
	}

	// Wrong token -> 401.
	req = httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/bad", bytes.NewReader([]byte(raw)))
	if _, err := p.inbound(ctx, &InboundInput{Ctx: humago.NewContext(nil, req, httptest.NewRecorder()), OrgSlug: "acme", Token: "bad"}); err == nil {
		t.Error("bad token must 401")
	}

	// X-Octarq-To overrides the RCPT target: mail lands in that mailbox.
	req = httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/tok", bytes.NewReader([]byte(raw)))
	req.Header.Set("X-Octarq-To", "bob@acme.example")
	out, err := p.inbound(ctx, &InboundInput{Ctx: humago.NewContext(nil, req, httptest.NewRecorder()), OrgSlug: "acme", Token: "tok"})
	if err != nil {
		t.Fatalf("inbound: %v", err)
	}
	if out.Body["stored"] != true {
		t.Errorf("inbound stored = %v, want true", out.Body["stored"])
	}
	var count int64
	db.Model(&Email{}).Where("mailbox_id = ?", mb.ID).Count(&count)
	if count != 1 {
		t.Errorf("inbound emails = %d, want 1", count)
	}
}

func TestExtractRawEmailMultipart(t *testing.T) {
	t.Parallel()

	// Multipart with a file part named "message".
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("message", "orig.eml")
	fw.Write([]byte("From: x\r\n\r\nbody"))
	w.Close()

	r := httptest.NewRequest(http.MethodPost, "/", &buf)
	r.Header.Set("Content-Type", w.FormDataContentType())
	b, err := extractRawEmail(r)
	if err != nil || string(b) != "From: x\r\n\r\nbody" {
		t.Errorf("multipart file extraction: %q err=%v", b, err)
	}

	// Multipart with a text field.
	buf.Reset()
	w = multipart.NewWriter(&buf)
	w.WriteField("raw", "From: y\r\n\r\ntextbody")
	w.Close()
	r = httptest.NewRequest(http.MethodPost, "/", &buf)
	r.Header.Set("Content-Type", w.FormDataContentType())
	b, err = extractRawEmail(r)
	if err != nil || string(b) != "From: y\r\n\r\ntextbody" {
		t.Errorf("multipart field extraction: %q err=%v", b, err)
	}

	// Non-multipart body is read directly.
	r2 := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("plain")))
	b2, err := extractRawEmail(r2)
	if err != nil || string(b2) != "plain" {
		t.Errorf("raw extraction: %q err=%v", b2, err)
	}
}

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	intmail "github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
)

// 1. Guard Test: Auth
func TestGenericInboundAuth(t *testing.T) {
	db, p := setupMailboxTestDB(t)

	org := models.Org{
		ID:           1,
		Name:         "Acme Corp",
		Slug:         "acme",
		InboundToken: "valid-secret-123",
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	ctx := context.Background()
	rawEML := []byte("From: alice@example.com\r\nTo: bob@acme.example\r\nSubject: Test\r\n\r\nHello")

	// Case 1.1: Missing token -> 401
	{
		req := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/raw/tok", bytes.NewReader(rawEML))
		input := &InboundGenericInput{
			Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
			OrgSlug: "acme",
			Token:   "",
		}
		_, err := p.inboundGeneric(ctx, input)
		if err == nil {
			t.Fatal("expected 401 error for missing token, got nil")
			return
		}
	}

	// Case 1.2: Wrong token -> 401
	{
		req := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/raw/tok", bytes.NewReader(rawEML))
		input := &InboundGenericInput{
			Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
			OrgSlug: "acme",
			Token:   "wrong-token",
		}
		_, err := p.inboundGeneric(ctx, input)
		if err == nil {
			t.Fatal("expected 401 error for wrong token, got nil")
			return
		}
	}

	// Case 1.3: Non-existent org slug with valid looking token -> 401 without leaking org existence
	{
		req := httptest.NewRequest(http.MethodPost, "/api/webhook/nonexistent-org/email/inbound/raw/tok", bytes.NewReader(rawEML))
		input := &InboundGenericInput{
			Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
			OrgSlug: "nonexistent-org",
			Token:   "valid-secret-123",
		}
		_, err := p.inboundGeneric(ctx, input)
		if err == nil {
			t.Fatal("expected 401 error for unknown org, got nil")
			return
		}
	}

	// Case 1.4: Valid token via X-Octarq-Token -> accepted
	{
		mb := Mailbox{OrgID: 1, Address: "bob@acme.example", Enabled: true}
		db.Create(&mb)

		req := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/raw/tok", bytes.NewReader(rawEML))
		input := &InboundGenericInput{
			Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
			OrgSlug: "acme",
			Token:   "valid-secret-123",
		}
		out, err := p.inboundGeneric(ctx, input)
		if err != nil {
			t.Fatalf("expected success with valid X-Octarq-Token, got: %v", err)
		}
		if stored, _ := out.Body["stored"].(bool); !stored {
			t.Fatalf("expected stored=true, got %#v", out.Body)
		}
	}
}

// 2. Guard Test: Ownership Validation (ownsMailHost must execute)
func TestGenericInboundOwnership(t *testing.T) {
	db, p := setupMailboxTestDB(t)

	p.getWorkspaceSetting = func(orgID uint, key string) string {
		if key == "mail.catch_all" {
			return "true"
		}
		return ""
	}

	// Org A (orgID=1) owns mymail.example
	orgA := models.Org{ID: 1, Slug: "org-a", InboundToken: "tok-a"}
	db.Create(&orgA)
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "mymail.example",
		ForMail: true,
		MailHosts: models.HostList{
			models.Host{Host: "mymail.example", Enabled: true},
		},
	})

	// Org B (orgID=2) owns victim.example
	orgB := models.Org{ID: 2, Slug: "org-b", InboundToken: "tok-b"}
	db.Create(&orgB)
	db.Create(&dns.Domain{
		OrgID:   2,
		Name:    "victim.example",
		ForMail: true,
		MailHosts: models.HostList{
			models.Host{Host: "victim.example", Enabled: true},
		},
	})

	ctx := context.Background()

	// Org A attempts generic inbound delivery to an unowned domain (victim.example)
	rawEML := []byte("From: attacker@external.com\r\nTo: someone@victim.example\r\nSubject: Exploit\r\n\r\nBody")
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/org-a/email/inbound/raw/tok", bytes.NewReader(rawEML))
	input := &InboundGenericInput{
		Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
		OrgSlug: "org-a",
		Token:   "tok-a",
	}

	out, err := p.inboundGeneric(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored, _ := out.Body["stored"].(bool); stored {
		t.Fatal("SECURITY ERROR: generic inbound delivered mail for an unowned domain! ownsMailHost check failed!")
	}

	// Verify no Email record was created in the database
	var count int64
	db.Model(&Email{}).Where("to_addr = ?", "someone@victim.example").Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 stored emails for unowned domain, found %d", count)
	}
}

// 3. Guard Test: Parsing real EML with attachment, non-ASCII subject, and multipart
func TestGenericInboundParsing(t *testing.T) {
	db, p := setupMailboxTestDB(t)

	org := models.Org{ID: 1, Slug: "acme", InboundToken: "sec123"}
	db.Create(&org)
	mb := Mailbox{OrgID: 1, Address: "user@example.com", Enabled: true}
	db.Create(&mb)

	// Complex EML with UTF-8 subject, multipart text + HTML, and attachment
	rawEML := []byte("From: Sender Name <sender@example.com>\r\n" +
		"To: user@example.com\r\n" +
		"Subject: =?UTF-8?B?5rWL6K+V6YKu5Lu2IFRlc3QgRW1haWwg8J+aow==?=\r\n" +
		"Message-ID: <msg-12345@example.com>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"_BOUNDARY_\"\r\n\r\n" +
		"--_BOUNDARY_\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Hello Text Body\r\n" +
		"--_BOUNDARY_\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n" +
		"<p>Hello HTML Body</p>\r\n" +
		"--_BOUNDARY_\r\n" +
		"Content-Type: application/pdf; name=\"document.pdf\"\r\n" +
		"Content-Disposition: attachment; filename=\"document.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		"JVBERi0xLjQK\r\n" +
		"--_BOUNDARY_--\r\n")

	ctx := context.Background()

	// Test 3.1: Raw EML POST with Authorization header
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/raw/tok", bytes.NewReader(rawEML))
	req.Header.Set("Content-Type", "message/rfc822")
	input := &InboundGenericInput{
		Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
		OrgSlug: "acme",
		Token:   "sec123",
	}

	out, err := p.inboundGeneric(ctx, input)
	if err != nil {
		t.Fatalf("inboundGeneric failed: %v", err)
	}
	emailIDVal, ok := out.Body["id"]
	if !ok {
		t.Fatalf("expected email ID in response body, got %#v", out.Body)
	}
	emailID := emailIDVal.(uint)

	var saved Email
	if err := db.First(&saved, emailID).Error; err != nil {
		t.Fatalf("failed to fetch saved email from DB: %v", err)
	}

	if saved.Subject != "测试邮件 Test Email 🚣" {
		t.Errorf("subject mismatch: got %q, want %q", saved.Subject, "测试邮件 Test Email 🚣")
	}
	if saved.FromAddr != "sender@example.com" {
		t.Errorf("from mismatch: got %q, want %q", saved.FromAddr, "sender@example.com")
	}
	if saved.ToAddr != "user@example.com" {
		t.Errorf("to mismatch: got %q, want %q", saved.ToAddr, "user@example.com")
	}
	if saved.Text != "Hello Text Body" {
		t.Errorf("text mismatch: got %q, want %q", saved.Text, "Hello Text Body")
	}
	if saved.HTML != "<p>Hello HTML Body</p>" {
		t.Errorf("html mismatch: got %q, want %q", saved.HTML, "<p>Hello HTML Body</p>")
	}
	if saved.MessageID != "msg-12345@example.com" {
		t.Errorf("messageID mismatch: got %q, want %q", saved.MessageID, "msg-12345@example.com")
	}

	var atts []intmail.Attachment
	if err := json.Unmarshal([]byte(saved.Attachments), &atts); err != nil || len(atts) != 1 {
		t.Fatalf("attachments parse mismatch: err=%v atts=%#v", err, atts)
	}
	if atts[0].Filename != "document.pdf" || atts[0].ContentType != "application/pdf" {
		t.Errorf("attachment mismatch: got %#v", atts[0])
	}

	// Test 3.2: Multipart Form POST (SendGrid Inbound Parse / Mailgun Routes format)
	var mpBuf bytes.Buffer
	writer := multipart.NewWriter(&mpBuf)
	part, _ := writer.CreateFormFile("email", "raw.eml")
	part.Write(rawEML)
	writer.Close()

	reqMP := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/raw/tok", &mpBuf)
	reqMP.Header.Set("Content-Type", writer.FormDataContentType())
	inputMP := &InboundGenericInput{
		Ctx:     humago.NewContext(nil, reqMP, httptest.NewRecorder()),
		OrgSlug: "acme",
		Token:   "sec123",
	}

	outMP, err := p.inboundGeneric(ctx, inputMP)
	if err != nil {
		t.Fatalf("inboundGeneric multipart failed: %v", err)
	}
	if stored, _ := outMP.Body["stored"].(bool); !stored {
		t.Fatalf("expected stored=true for multipart POST, got %#v", outMP.Body)
	}
}

// 4. Guard Test: Equivalence between Cloudflare Route and Generic Route
func TestInboundEquivalence(t *testing.T) {
	db, p := setupMailboxTestDB(t)

	org := models.Org{ID: 1, Slug: "acme", InboundToken: "shared-token"}
	db.Create(&org)

	mb1 := Mailbox{OrgID: 1, Address: "cf@example.com", Enabled: true}
	db.Create(&mb1)
	mb2 := Mailbox{OrgID: 1, Address: "gen@example.com", Enabled: true}
	db.Create(&mb2)

	rawCF := []byte("From: tester@example.com\r\nTo: cf@example.com\r\nSubject: Equivalence Test\r\nMessage-ID: <eq-1@example.com>\r\n\r\nEquivalence Body")
	rawGen := []byte("From: tester@example.com\r\nTo: gen@example.com\r\nSubject: Equivalence Test\r\nMessage-ID: <eq-1@example.com>\r\n\r\nEquivalence Body")

	ctx := context.Background()

	// Path 1: Cloudflare route
	reqCF := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/shared-token", bytes.NewReader(rawCF))
	inputCF := &InboundInput{
		Ctx:     humago.NewContext(nil, reqCF, httptest.NewRecorder()),
		OrgSlug: "acme",
		Token:   "shared-token",
	}
	outCF, err := p.inbound(ctx, inputCF)
	if err != nil {
		t.Fatalf("Cloudflare inbound route error: %v", err)
	}

	// Path 2: Generic route
	reqGen := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/raw/tok", bytes.NewReader(rawGen))
	inputGen := &InboundGenericInput{
		Ctx:     humago.NewContext(nil, reqGen, httptest.NewRecorder()),
		OrgSlug: "acme",
		Token:   "shared-token",
	}
	outGen, err := p.inboundGeneric(ctx, inputGen)
	if err != nil {
		t.Fatalf("Generic inbound route error: %v", err)
	}

	idCF := outCF.Body["id"].(uint)
	idGen := outGen.Body["id"].(uint)

	var emailCF, emailGen Email
	db.First(&emailCF, idCF)
	db.First(&emailGen, idGen)

	if emailCF.FromAddr != emailGen.FromAddr {
		t.Errorf("From mismatch: CF=%q Gen=%q", emailCF.FromAddr, emailGen.FromAddr)
	}
	if emailCF.Subject != emailGen.Subject {
		t.Errorf("Subject mismatch: CF=%q Gen=%q", emailCF.Subject, emailGen.Subject)
	}
	if emailCF.Text != emailGen.Text {
		t.Errorf("Text mismatch: CF=%q Gen=%q", emailCF.Text, emailGen.Text)
	}
	if emailCF.HTML != emailGen.HTML {
		t.Errorf("HTML mismatch: CF=%q Gen=%q", emailCF.HTML, emailGen.HTML)
	}
	if emailCF.MessageID != emailGen.MessageID {
		t.Errorf("MessageID mismatch: CF=%q Gen=%q", emailCF.MessageID, emailGen.MessageID)
	}
	if emailCF.Attachments != emailGen.Attachments {
		t.Errorf("Attachments mismatch: CF=%q Gen=%q", emailCF.Attachments, emailGen.Attachments)
	}
}

package mail

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestMailGet_BodyRetrievalAndFences(t *testing.T) {
	db, p := setupMailboxTestDB(t)

	mb1 := Mailbox{OrgID: 1, Address: "alice@org1.test", Enabled: true}
	db.Create(&mb1)

	atts := []AttachmentDTO{
		{Filename: "invoice.pdf", ContentType: "application/pdf", Size: 1024},
		{Filename: "sheet.xlsx", ContentType: "application/vnd.ms-excel", Size: 2048},
	}
	attsJSON, _ := json.Marshal(atts)

	e1 := Email{
		MailboxID:   mb1.ID,
		MessageID:   "<msg1@org1.test>",
		FromAddr:    "vendor@billing.com",
		ToAddr:      "alice@org1.test, bob@org1.test",
		Subject:     "Monthly Invoice #1024",
		Text:        "Hello Alice,\nYour invoice of $49.00 is ready.\nVisit http://example.com/reset-password/secret1234567890abcdef for details.",
		Attachments: string(attsJSON),
		ReceivedAt:  time.Now(),
	}
	db.Create(&e1)

	ctx1 := plugin.WithOrgID(context.Background(), 1)
	ctx2 := plugin.WithOrgID(context.Background(), 2)

	// 1. Successful read as Org 1
	out, err := p.handleReadEmailBody(ctx1, ReadEmailInput{ID: e1.ID})
	if err != nil {
		t.Fatalf("handleReadEmailBody failed: %v", err)
	}

	if out.ID != e1.ID {
		t.Errorf("expected ID %d, got %d", e1.ID, out.ID)
	}
	if out.From != "vendor@billing.com" {
		t.Errorf("expected from 'vendor@billing.com', got %q", out.From)
	}
	if len(out.To) != 2 || out.To[0] != "alice@org1.test" || out.To[1] != "bob@org1.test" {
		t.Errorf("unexpected recipients: %v", out.To)
	}
	if out.Subject != "Monthly Invoice #1024" {
		t.Errorf("expected subject 'Monthly Invoice #1024', got %q", out.Subject)
	}

	// Verify <untrusted> fence
	if !strings.HasPrefix(out.TextBody, "<untrusted>\n") || !strings.HasSuffix(out.TextBody, "\n</untrusted>") {
		t.Errorf("TextBody must be fenced with <untrusted>...</untrusted>, got: %q", out.TextBody)
	}

	// Verify token redaction inside fence
	if strings.Contains(out.TextBody, "secret1234567890abcdef") {
		t.Errorf("TextBody contains unredacted sensitive token: %q", out.TextBody)
	}

	// Verify attachment metadata
	if len(out.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(out.Attachments))
	}
	if out.Attachments[0].Filename != "invoice.pdf" || out.Attachments[0].Size != 1024 {
		t.Errorf("unexpected attachment 0: %+v", out.Attachments[0])
	}

	// 2. HTML-only body fallback with script tag stripping
	eHTML := Email{
		MailboxID:  mb1.ID,
		FromAddr:   "marketing@vendor.com",
		ToAddr:     "alice@org1.test",
		Subject:    "HTML News",
		HTML:       `<html><head><script>alert('xss');</script><style>.c { color: red; }</style></head><body><h1>Welcome</h1><p>Important update.</p></body></html>`,
		ReceivedAt: time.Now(),
	}
	db.Create(&eHTML)

	outHTML, err := p.handleReadEmailBody(ctx1, ReadEmailInput{EmailID: eHTML.ID})
	if err != nil {
		t.Fatalf("handleReadEmailBody for HTML-only failed: %v", err)
	}
	if strings.Contains(outHTML.TextBody, "alert('xss')") || strings.Contains(outHTML.TextBody, "<script>") {
		t.Errorf("HTML scripts were not safely stripped: %q", outHTML.TextBody)
	}
	if !strings.Contains(outHTML.TextBody, "Welcome") || !strings.Contains(outHTML.TextBody, "Important update.") {
		t.Errorf("expected converted plain text in TextBody: %q", outHTML.TextBody)
	}
	if !strings.HasPrefix(outHTML.TextBody, "<untrusted>\n") {
		t.Errorf("HTML TextBody must also be fenced: %q", outHTML.TextBody)
	}

	// 3. MCP tool invocation
	mcpRes, _, mcpErr := p.mcpMailGet(ctx1, nil, ReadEmailInput{EmailIDCamel: e1.ID})
	if mcpErr != nil {
		t.Fatalf("mcpMailGet failed: %v", mcpErr)
	}
	if len(mcpRes.Content) == 0 {
		t.Fatal("expected non-empty MCP response content")
	}

	// 4. Cross-tenant isolation: Org 2 must NOT be able to read Org 1's email
	if _, err := p.handleReadEmailBody(ctx2, ReadEmailInput{ID: e1.ID}); err == nil {
		t.Fatal("security violation: Org 2 accessed Org 1's email!")
	}
}

func TestMailDraftCreate(t *testing.T) {
	db, p := setupMailboxTestDB(t)

	mb := Mailbox{OrgID: 1, Address: "alice@org1.test", Enabled: true}
	db.Create(&mb)

	ctx1 := plugin.WithOrgID(context.Background(), 1)
	ctx2 := plugin.WithOrgID(context.Background(), 2)

	// 1. Missing recipient should fail
	_, err := p.handleCreateDraft(ctx1, CreateDraftInput{
		Subject: "Draft without recipient",
		Body:    "Testing draft creation",
	})
	if err == nil {
		t.Fatal("expected error when recipient is empty")
	}

	// 2. Successful draft creation
	out, err := p.handleCreateDraft(ctx1, CreateDraftInput{
		MailboxID:   mb.ID,
		To:          "partner@example.com",
		CC:          "manager@org1.test",
		Subject:     "Project Proposal Draft",
		Body:        "Please find the initial outline below...",
		InReplyToID: 42,
	})
	if err != nil {
		t.Fatalf("handleCreateDraft failed: %v", err)
	}

	if out.ID == 0 {
		t.Fatal("expected positive draft ID")
	}
	if out.To != "partner@example.com" {
		t.Errorf("expected to 'partner@example.com', got %q", out.To)
	}
	if out.CC != "manager@org1.test" {
		t.Errorf("expected CC 'manager@org1.test', got %q", out.CC)
	}
	if out.Subject != "Project Proposal Draft" {
		t.Errorf("expected subject 'Project Proposal Draft', got %q", out.Subject)
	}

	// Verify persistence in mail_drafts table
	var savedDraft MailDraft
	if err := db.Where("id = ?", out.ID).First(&savedDraft).Error; err != nil {
		t.Fatalf("draft not persisted in mail_drafts: %v", err)
	}
	if savedDraft.OrgID != 1 {
		t.Errorf("expected OrgID 1 in mail_drafts, got %d", savedDraft.OrgID)
	}
	if savedDraft.InReplyToID != 42 {
		t.Errorf("expected InReplyToID 42, got %d", savedDraft.InReplyToID)
	}

	// 3. MCP tool invocation
	_, _, err = p.mcpMailDraftCreate(ctx1, nil, CreateDraftInput{
		To:      "mcp@test.com",
		Subject: "MCP Draft",
		Body:    "Created via MCP tool",
	})
	if err != nil {
		t.Fatalf("mcpMailDraftCreate failed: %v", err)
	}

	// 4. Unauthorized (no org)
	if _, err := p.handleCreateDraft(context.Background(), CreateDraftInput{To: "a@b.com"}); err == nil {
		t.Fatal("expected unauthorized error when org is absent")
	}

	// 5. Cross-org mailbox rejection: Org 2 trying to use Org 1's mailbox
	_, errOrg2 := p.handleCreateDraft(ctx2, CreateDraftInput{
		MailboxID: mb.ID,
		To:        "partner@example.com",
		Subject:   "Cross-tenant attempt",
		Body:      "Should fail",
	})
	if errOrg2 == nil {
		t.Fatal("security violation: Org 2 used Org 1's mailbox for draft!")
	}
}

func TestMailSendReply_DeclarativeContractAndHITL(t *testing.T) {
	db, p := setupMailboxTestDB(t)

	// Verify endpoint specifications
	var sendSpec plugin.Endpoint
	var getSpec plugin.Endpoint
	var draftSpec plugin.Endpoint

	pctx := &plugin.Context{
		DB: db,
		RegisterEndpoint: func(s any) error {
			if ep, ok := s.(plugin.Endpoint); ok {
				switch ep.EndpointName() {
				case "send_email", "mail_send_reply":
					sendSpec = ep
				case "read_email_body", "mail_get":
					getSpec = ep
				case "create_email_draft", "mail_draft_create":
					draftSpec = ep
				}
			}
			return nil
		},
	}
	p.Mount(nil, pctx)

	if sendSpec == nil {
		t.Fatal("send_email / mail_send_reply endpoint was not registered")
	}
	if getSpec == nil {
		t.Fatal("read_email_body / mail_get endpoint was not registered")
	}
	if draftSpec == nil {
		t.Fatal("create_email_draft / mail_draft_create endpoint was not registered")
	}

	// 1. Verify Risk Levels and HITL Approval Gate
	if getSpec.EndpointRisk() != plugin.RiskLevelRead {
		t.Errorf("expected RiskLevelRead for read_email_body, got %q", getSpec.EndpointRisk())
	}
	if draftSpec.EndpointRisk() != plugin.RiskLevelWrite {
		t.Errorf("expected RiskLevelWrite for create_email_draft, got %q", draftSpec.EndpointRisk())
	}
	if sendSpec.EndpointRisk() != plugin.RiskLevelDestructive {
		t.Errorf("expected RiskLevelDestructive for send_email, got %q", sendSpec.EndpointRisk())
	}
	if !sendSpec.EndpointRequireApproval() {
		t.Errorf("send_email must require HITL approval before execution")
	}

	ctx1 := plugin.WithOrgID(context.Background(), 1)

	// 2. Missing recipient fails closed
	_, err := p.handleSendEmail(ctx1, SendEmailToolInput{
		Subject: "Hello",
		Body:    "Body",
	})
	if err == nil {
		t.Fatal("expected error when recipients are missing")
	}

	// 3. Suppression check fails closed
	db.Create(&MailSuppression{OrgID: 1, Address: "blocked@evil.test", Reason: "hard_bounce"})
	_, errSuppressed := p.handleSendEmail(ctx1, SendEmailToolInput{
		To:      []string{"blocked@evil.test"},
		Subject: "Spam",
		Body:    "Text",
	})
	if errSuppressed == nil {
		t.Fatal("expected suppression error when sending to blocked address")
	}

	// 4. Missing SMTP sender fails closed
	_, errNoSMTP := p.handleSendEmail(ctx1, SendEmailToolInput{
		To:      []string{"valid@example.com"},
		Subject: "Test",
		Body:    "Hello",
	})
	if errNoSMTP == nil {
		t.Fatal("expected error when no SMTP sender is configured")
	}
}

package mail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestRegisterMCPRegistersMailTools(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	New().RegisterMCP(server)

	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"list_mailboxes", "list_emails", "get_latest_otp", "get_email_summary", "get_email_content"} {
		if !names[want] {
			t.Errorf("tool %s not registered; got %v", want, names)
		}
	}
}

func TestMCPListMailboxes(t *testing.T) {
	db, p := setupMailboxTestDB(t)
	db.Where("1 = 1").Delete(&Mailbox{})
	db.Where("1 = 1").Delete(&Email{})

	db.Create(&Mailbox{OrgID: 1, Address: "a@example.com", Enabled: true})
	db.Create(&Mailbox{OrgID: 2, Address: "b@example.com", Enabled: true})

	ctx := plugin.WithOrgID(context.Background(), 1)

	if _, _, err := p.mcpListMailboxes(context.Background(), nil, listMailboxesInput{}); err == nil {
		t.Fatal("no org in context must be refused")
		return
	}

	res, out, err := p.mcpListMailboxes(ctx, nil, listMailboxesInput{})
	if err != nil {
		t.Fatalf("mcpListMailboxes: %v", err)
	}
	list, ok := out.([]mailboxOut)
	if !ok {
		t.Fatalf("unexpected output type %T", out)
	}
	if len(list) != 1 || list[0].Address != "a@example.com" {
		t.Errorf("org-scoped mailbox list wrong: %+v", list)
	}
	text := ""
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		text = tc.Text
	}
	if !strings.Contains(text, "a@example.com") {
		t.Errorf("result missing address: %s", text)
	}

	// Unread count is computed per mailbox.
	mb := Mailbox{OrgID: 1, Address: "unread@example.com", Enabled: true}
	db.Create(&mb)
	db.Create(&Email{MailboxID: mb.ID, Read: false, ReceivedAt: time.Now()})
	_, out2, err := p.mcpListMailboxes(ctx, nil, listMailboxesInput{})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	found := false
	for _, mb := range out2.([]mailboxOut) {
		if mb.Address == "unread@example.com" && mb.Unread == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("unread count missing: %+v", out2)
	}
}

func TestMCPListEmails(t *testing.T) {
	db, p := setupMailboxTestDB(t)
	db.Where("1 = 1").Delete(&Mailbox{})
	db.Where("1 = 1").Delete(&Email{})

	mb := Mailbox{OrgID: 1, Address: "in@example.com", Enabled: true}
	db.Create(&mb)
	mb2 := Mailbox{OrgID: 1, Address: "in2@example.com", Enabled: true}
	db.Create(&mb2)
	db.Create(&Email{MailboxID: mb.ID, FromAddr: "x@y.z", Subject: "one", Read: false, ReceivedAt: time.Now()})
	db.Create(&Email{MailboxID: mb.ID, FromAddr: "x@y.z", Subject: "two", Read: true, ReceivedAt: time.Now()})
	db.Create(&Email{MailboxID: mb2.ID, FromAddr: "x@y.z", Subject: "three", Read: false, ReceivedAt: time.Now()})

	ctx := plugin.WithOrgID(context.Background(), 1)

	if _, _, err := p.mcpListEmails(context.Background(), nil, listEmailsInput{}); err == nil {
		t.Fatal("no org in context must be refused")
		return
	}

	_, out, err := p.mcpListEmails(ctx, nil, listEmailsInput{})
	if err != nil {
		t.Fatalf("mcpListEmails: %v", err)
	}
	if len(out.([]emailOut)) != 3 {
		t.Errorf("email count = %d, want 3", len(out.([]emailOut)))
	}

	// Mailbox filter narrows; unread-only filters further.
	_, onMB, err := p.mcpListEmails(ctx, nil, listEmailsInput{MailboxID: mb.ID})
	if err != nil {
		t.Fatalf("mailbox filter: %v", err)
	}
	if len(onMB.([]emailOut)) != 2 {
		t.Errorf("mailbox-filtered count = %d, want 2", len(onMB.([]emailOut)))
	}
	_, unread, err := p.mcpListEmails(ctx, nil, listEmailsInput{MailboxID: mb.ID, UnreadOnly: true})
	if err != nil {
		t.Fatalf("unread filter: %v", err)
	}
	if len(unread.([]emailOut)) != 1 || unread.([]emailOut)[0].Subject != "one" {
		t.Errorf("unread filter: %+v", unread)
	}

	// Limit bounds without error.
	if _, _, err := p.mcpListEmails(ctx, nil, listEmailsInput{Limit: -1}); err != nil {
		t.Errorf("default limit: %v", err)
	}
	if _, _, err := p.mcpListEmails(ctx, nil, listEmailsInput{Limit: 500}); err != nil {
		t.Errorf("clamped limit: %v", err)
	}

	// An org with no mailboxes yields an empty list, not an error.
	db.Where("1 = 1").Delete(&Mailbox{})
	_, emptyOrg, err := p.mcpListEmails(ctx, nil, listEmailsInput{})
	if err != nil {
		t.Fatalf("empty-org call: %v", err)
	}
	if len(emptyOrg.([]emailOut)) != 0 {
		t.Errorf("empty org must yield no emails, got %d", len(emptyOrg.([]emailOut)))
	}
}

func TestMCPExportMailboxesAndEmails(t *testing.T) {
	db, p := setupMailboxTestDB(t)
	db.Where("1 = 1").Delete(&Mailbox{})
	db.Where("1 = 1").Delete(&Email{})

	mb := Mailbox{OrgID: 1, Address: "export@example.com", Enabled: true}
	db.Create(&mb)
	db.Create(&Email{MailboxID: mb.ID, FromAddr: "x@y.z", Subject: "sv", Read: false, ReceivedAt: time.Now()})

	boxes, err := p.mcpExportMailboxes(context.Background(), 1)
	if err != nil {
		t.Fatalf("mcpExportMailboxes: %v", err)
	}
	if len(boxes.([]Mailbox)) != 1 {
		t.Errorf("exported mailboxes = %d, want 1", len(boxes.([]Mailbox)))
	}

	emails, err := p.mcpExportEmails(context.Background(), 1)
	if err != nil {
		t.Fatalf("mcpExportEmails: %v", err)
	}
	outs, ok := emails.([]emailOut)
	if !ok || len(outs) != 1 || outs[0].Subject != "sv" {
		t.Errorf("exported emails = %+v", emails)
	}

	// Another org has no mailboxes -> export is empty.
	none, err := p.mcpExportEmails(context.Background(), 99)
	if err != nil {
		t.Fatalf("empty export: %v", err)
	}
	if len(none.([]emailOut)) != 0 {
		t.Errorf("empty org export = %d, want 0", len(none.([]emailOut)))
	}
}

func TestMCPGetLatestOTP(t *testing.T) {
	db, p := setupMailboxTestDB(t)
	db.Where("1 = 1").Delete(&Mailbox{})
	db.Where("1 = 1").Delete(&Email{})

	// 1. Refuses call without org context
	if _, _, err := p.mcpGetLatestOTP(context.Background(), nil, getLatestOTPInput{}); err == nil {
		t.Fatal("expected error without org in context")
	}

	ctx1 := plugin.WithOrgID(context.Background(), 1)
	ctx2 := plugin.WithOrgID(context.Background(), 2)

	// 2. Org with no mailboxes
	res, out, err := p.mcpGetLatestOTP(ctx1, nil, getLatestOTPInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	otpOut := out.(otpResult)
	if otpOut.Found {
		t.Errorf("expected Found = false for empty org")
	}

	// 3. Mailbox exists but no emails
	mb1 := Mailbox{OrgID: 1, Address: "agent@corp.com", Enabled: true}
	db.Create(&mb1)
	mb2 := Mailbox{OrgID: 2, Address: "other@corp.com", Enabled: true}
	db.Create(&mb2)

	_, outNoEmail, err := p.mcpGetLatestOTP(ctx1, nil, getLatestOTPInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outNoEmail.(otpResult).Found {
		t.Errorf("expected Found = false when no emails exist")
	}

	// 4. Email received 15 minutes ago (older than 10m window) -> ignored
	db.Create(&Email{
		MailboxID:  mb1.ID,
		FromAddr:   "auth@saas.com",
		Subject:    "Your verification code is 111111",
		ReceivedAt: time.Now().Add(-15 * time.Minute),
		Read:       false,
	})
	_, outOld, _ := p.mcpGetLatestOTP(ctx1, nil, getLatestOTPInput{})
	if outOld.(otpResult).Found {
		t.Errorf("expected Found = false for email >10m ago")
	}

	// 5. Email is already read -> ignored
	db.Create(&Email{
		MailboxID:  mb1.ID,
		FromAddr:   "auth@saas.com",
		Subject:    "Your verification code is 222222",
		ReceivedAt: time.Now().Add(-2 * time.Minute),
		Read:       true,
	})
	_, outRead, _ := p.mcpGetLatestOTP(ctx1, nil, getLatestOTPInput{})
	if outRead.(otpResult).Found {
		t.Errorf("expected Found = false for read email")
	}

	// 6. Recent unread email with OTP arrived 2 minutes ago
	db.Create(&Email{
		MailboxID:  mb1.ID,
		FromAddr:   "service@saas.com",
		ToAddr:     "agent@corp.com",
		Subject:    "【SaaS】您的验证码为 783921，请于10分钟内输入",
		ReceivedAt: time.Now().Add(-2 * time.Minute),
		Read:       false,
	})

	// Call without filters
	_, outFound, err := p.mcpGetLatestOTP(ctx1, nil, getLatestOTPInput{})
	if err != nil {
		t.Fatalf("mcpGetLatestOTP failed: %v", err)
	}
	resFound := outFound.(otpResult)
	if !resFound.Found || resFound.OTP != "783921" {
		t.Errorf("expected OTP 783921, got %+v", resFound)
	}

	// Call with mailbox_id filter
	_, outWithID, _ := p.mcpGetLatestOTP(ctx1, nil, getLatestOTPInput{MailboxID: mb1.ID})
	if outWithID.(otpResult).OTP != "783921" {
		t.Errorf("expected OTP with MailboxID filter")
	}

	// Call with camelCase mailboxId filter
	_, outWithIDCamel, _ := p.mcpGetLatestOTP(ctx1, nil, getLatestOTPInput{MailboxIDCamel: mb1.ID})
	if outWithIDCamel.(otpResult).OTP != "783921" {
		t.Errorf("expected OTP with MailboxIDCamel filter")
	}

	// Call with mailbox_address filter
	_, outWithAddr, _ := p.mcpGetLatestOTP(ctx1, nil, getLatestOTPInput{MailboxAddress: "AGENT@corp.com"})
	if outWithAddr.(otpResult).OTP != "783921" {
		t.Errorf("expected OTP with MailboxAddress filter")
	}

	// Call with mismatched mailbox_address
	_, outMismatch, _ := p.mcpGetLatestOTP(ctx1, nil, getLatestOTPInput{MailboxAddress: "nonexistent@corp.com"})
	if outMismatch.(otpResult).Found {
		t.Errorf("expected Found = false for mismatched mailbox_address")
	}

	// 7. Security: Tenant isolation (Org 2 cannot read Org 1's unread OTP)
	_, outOrg2, _ := p.mcpGetLatestOTP(ctx2, nil, getLatestOTPInput{})
	if outOrg2.(otpResult).Found {
		t.Errorf("security violation: Org 2 must not see Org 1's OTP")
	}

	_ = res
}

func TestMCPGetEmailContentAndSummary(t *testing.T) {
	db, p := setupMailboxTestDB(t)
	db.Where("1 = 1").Delete(&Mailbox{})
	db.Where("1 = 1").Delete(&Email{})

	ctx1 := plugin.WithOrgID(context.Background(), 1)
	ctx2 := plugin.WithOrgID(context.Background(), 2)

	// Refusal without org context
	if _, _, err := p.mcpGetEmailContent(context.Background(), nil, getEmailContentInput{EmailID: 1}); err == nil {
		t.Fatal("expected error without org in context")
	}
	if _, _, err := p.mcpGetEmailSummary(context.Background(), nil, getEmailSummaryInput{EmailID: 1}); err == nil {
		t.Fatal("expected error without org in context")
	}

	// Missing email_id
	if _, _, err := p.mcpGetEmailContent(ctx1, nil, getEmailContentInput{}); err == nil {
		t.Fatal("expected error for missing email_id")
	}

	// Mailbox for org 1
	mb1 := Mailbox{OrgID: 1, Address: "worker@corp.com", Enabled: true}
	db.Create(&mb1)

	// Create email with password reset token and inline base64 image
	bodyWithSensitive := "Hello,\nYour verification code is 998877.\nTo reset password, click https://app.example.com/reset-password?token=secret1234567890abcdef\nInline avatar: data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==\nDone."
	email1 := Email{
		MailboxID:  mb1.ID,
		FromAddr:   "support@saas.com",
		ToAddr:     "worker@corp.com",
		Subject:    "Security Notice: Code 998877",
		Text:       bodyWithSensitive,
		ReceivedAt: time.Now(),
	}
	db.Create(&email1)

	// 1. Get email content as Org 1
	_, outContent, err := p.mcpGetEmailContent(ctx1, nil, getEmailContentInput{EmailID: email1.ID})
	if err != nil {
		t.Fatalf("mcpGetEmailContent failed: %v", err)
	}
	contentRes := outContent.(emailContentOut)
	if strings.Contains(contentRes.Text, "secret1234567890abcdef") {
		t.Errorf("token was not redacted from email content: %s", contentRes.Text)
	}
	if !strings.Contains(contentRes.Text, "token=[REDACTED_TOKEN]") {
		t.Errorf("expected redacted token placeholder, got: %s", contentRes.Text)
	}
	if strings.Contains(contentRes.Text, "iVBORw0KGgoAAAANSUhEUg") {
		t.Errorf("base64 image was not stripped: %s", contentRes.Text)
	}
	if !strings.Contains(contentRes.Text, "[inline image truncated]") {
		t.Errorf("expected [inline image truncated], got: %s", contentRes.Text)
	}
	if contentRes.Truncated {
		t.Errorf("normal sized email should not be truncated")
	}

	// 2. Test camelCase emailId
	_, outCamel, err := p.mcpGetEmailContent(ctx1, nil, getEmailContentInput{EmailIDCamel: email1.ID})
	if err != nil || outCamel.(emailContentOut).ID != email1.ID {
		t.Errorf("failed fetching with EmailIDCamel")
	}

	// 3. Security: Org 2 cannot read Org 1's email content
	_, _, errOrg2 := p.mcpGetEmailContent(ctx2, nil, getEmailContentInput{EmailID: email1.ID})
	if errOrg2 == nil {
		t.Errorf("security violation: Org 2 must not read Org 1's email content")
	}

	// 4. Get email summary as Org 1
	_, outSummary, err := p.mcpGetEmailSummary(ctx1, nil, getEmailSummaryInput{EmailID: email1.ID})
	if err != nil {
		t.Fatalf("mcpGetEmailSummary failed: %v", err)
	}
	sumRes := outSummary.(emailSummaryOut)
	if sumRes.OTP != "998877" {
		t.Errorf("expected OTP 998877 in summary, got %q", sumRes.OTP)
	}
	if sumRes.Category != "otp" {
		t.Errorf("expected category 'otp', got %q", sumRes.Category)
	}
	if strings.Contains(sumRes.Summary, "secret1234567890abcdef") {
		t.Errorf("summary contained sensitive token: %s", sumRes.Summary)
	}

	// 5. Test HTML-only email fallback
	emailHTMLOnly := Email{
		MailboxID:  mb1.ID,
		FromAddr:   "newsletter@saas.com",
		ToAddr:     "worker@corp.com",
		Subject:    "Weekly Update",
		HTML:       "<html><body><p>Hello world from HTML only!</p></body></html>",
		ReceivedAt: time.Now(),
	}
	db.Create(&emailHTMLOnly)

	_, outHTML, err := p.mcpGetEmailContent(ctx1, nil, getEmailContentInput{EmailID: emailHTMLOnly.ID})
	if err != nil {
		t.Fatalf("failed fetching HTML only email: %v", err)
	}
	if !strings.Contains(outHTML.(emailContentOut).Text, "Hello world from HTML only!") {
		t.Errorf("expected stripped HTML text, got: %s", outHTML.(emailContentOut).Text)
	}

	// 6. Org 2 cannot read Org 1's email summary
	if _, _, err := p.mcpGetEmailSummary(ctx2, nil, getEmailSummaryInput{EmailID: email1.ID}); err == nil {
		t.Errorf("security violation: Org 2 must not read Org 1's email summary")
	}
}

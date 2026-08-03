package mail

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func TestMailPluginMetaPurgeExportAndMCP(t *testing.T) {
	t.Parallel()

	db, p := setupMailboxTestDB(t)
	ctx := context.Background()

	// 1. Metadata
	if p.Name() != "mail" {
		t.Errorf("Name = %q", p.Name())
	}
	desc := p.Describe()
	if desc.Title != "Mail" {
		t.Errorf("Describe Title = %q", desc.Title)
	}
	if len(p.Models()) == 0 {
		t.Error("Models empty")
	}
	if len(p.Menus()) == 0 {
		t.Error("Menus empty")
	}
	if len(p.Actions()) == 0 {
		t.Error("Actions empty")
	}
	if p.HelpDocsFS() == nil {
		t.Error("HelpDocsFS is nil")
	}

	// 2. Data population and Purge / Export
	mb := Mailbox{OrgID: 1, Address: "test@example.com"}
	db.Create(&mb)
	em := Email{MailboxID: mb.ID, Subject: "Test Email", Text: "Body"}
	db.Create(&em)
	smtp := SMTPSender{OrgID: 1, Host: "smtp.example.com"}
	db.Create(&smtp)

	exp := p.exportData(1)
	if exp == nil || len(exp["mailboxes"].([]Mailbox)) == 0 {
		t.Errorf("exportData failed: %+v", exp)
	}

	// MCP Export
	mbExp, err := p.mcpExportMailboxes(ctx, 1)
	if err != nil || mbExp == nil {
		t.Errorf("mcpExportMailboxes failed: %v, %+v", err, mbExp)
	}
	emExp, err := p.mcpExportEmails(ctx, 1)
	if err != nil || emExp == nil {
		t.Errorf("mcpExportEmails failed: %v, %+v", err, emExp)
	}

	// MCP Tool Calls with context org 1
	ctxOrg := plugin.WithOrgID(ctx, 1)
	_, mbsOut, err := p.mcpListMailboxes(ctxOrg, nil, listMailboxesInput{})
	if err != nil || mbsOut == nil {
		t.Errorf("mcpListMailboxes error=%v, out=%+v", err, mbsOut)
	}

	_, emsOut, err := p.mcpListEmails(ctxOrg, nil, listEmailsInput{})
	if err != nil || emsOut == nil {
		t.Errorf("mcpListEmails error=%v, out=%+v", err, emsOut)
	}

	// MCP Tool Calls without org context -> error
	if _, _, err := p.mcpListMailboxes(ctx, nil, listMailboxesInput{}); err == nil {
		t.Error("expected error when calling mcpListMailboxes with no org")
	}

	// Purge
	if err := p.purge(1); err != nil {
		t.Fatalf("purge error: %v", err)
	}
	exp2 := p.exportData(1)
	if len(exp2["mailboxes"].([]Mailbox)) != 0 {
		t.Errorf("expected 0 mailboxes after purge, got %v", exp2)
	}
}

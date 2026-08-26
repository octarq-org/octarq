package mail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/plugin"
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
	for _, want := range []string{"list_mailboxes", "list_emails"} {
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

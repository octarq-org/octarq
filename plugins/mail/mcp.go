package mail

import (
	"context"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/plugin"
)

type listMailboxesInput struct{}

type mailboxOut struct {
	ID      uint   `json:"id"`
	Address string `json:"address"`
	Enabled bool   `json:"enabled"`
	Unread  int64  `json:"unread"`
}

type listEmailsInput struct {
	MailboxID  uint `json:"mailboxId,omitempty"`
	Limit      int  `json:"limit,omitempty"`
	UnreadOnly bool `json:"unreadOnly,omitempty"`
}

type emailOut struct {
	ID         uint      `json:"id"`
	MailboxID  uint      `json:"mailboxId"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Subject    string    `json:"subject"`
	Read       bool      `json:"read"`
	ReceivedAt time.Time `json:"receivedAt"`
}

func (p *Plugin) RegisterMCP(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_mailboxes",
		Description: "List email mailboxes with their unread counts.",
	}, p.mcpListMailboxes)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_emails",
		Description: "List recently received emails (subject, from, to, date) — optionally for one mailbox. Bodies are not returned; use query_db_readonly for more, or get the full message via the dashboard.",
	}, p.mcpListEmails)
}

func (p *Plugin) mcpListMailboxes(ctx context.Context, _ *mcp.CallToolRequest, _ listMailboxesInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		orgID = 1
	}
	var mbs []Mailbox
	if err := p.db.WithContext(ctx).
		Where("owner_id = ?", orgID).
		Order("address ASC").Find(&mbs).Error; err != nil {
		return nil, nil, err
	}
	out := make([]mailboxOut, 0, len(mbs))
	for _, mb := range mbs {
		var unread int64
		p.db.WithContext(ctx).Model(&Email{}).
			Where("mailbox_id = ? AND read = ?", mb.ID, false).Count(&unread)
		out = append(out, mailboxOut{ID: mb.ID, Address: mb.Address, Enabled: mb.Enabled, Unread: unread})
	}
	return jsonResult(out)
}

func (p *Plugin) mcpListEmails(ctx context.Context, _ *mcp.CallToolRequest, in listEmailsInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		orgID = 1
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 30
	} else if limit > 200 {
		limit = 200
	}
	var mailboxIDs []uint
	p.db.WithContext(ctx).Model(&Mailbox{}).
		Where("owner_id = ?", orgID).Pluck("id", &mailboxIDs)
	if len(mailboxIDs) == 0 {
		return jsonResult([]emailOut{})
	}

	q := p.db.WithContext(ctx).Model(&Email{}).
		Where("mailbox_id IN ?", mailboxIDs).
		Order("received_at DESC").
		Limit(limit)
	if in.MailboxID != 0 {
		q = q.Where("mailbox_id = ?", in.MailboxID)
	}
	if in.UnreadOnly {
		q = q.Where("read = ?", false)
	}
	var emails []Email
	if err := q.Find(&emails).Error; err != nil {
		return nil, nil, err
	}
	out := make([]emailOut, 0, len(emails))
	for _, e := range emails {
		out = append(out, emailOut{
			ID: e.ID, MailboxID: e.MailboxID, From: e.FromAddr, To: e.ToAddr,
			Subject: e.Subject, Read: e.Read, ReceivedAt: e.ReceivedAt,
		})
	}
	return jsonResult(out)
}

func (p *Plugin) mcpExportMailboxes(ctx context.Context, orgID uint) (any, error) {
	var v []Mailbox
	if err := p.db.WithContext(ctx).Where("owner_id = ?", orgID).Find(&v).Error; err != nil {
		return nil, err
	}
	return v, nil
}

func (p *Plugin) mcpExportEmails(ctx context.Context, orgID uint) (any, error) {
	var mailboxIDs []uint
	p.db.WithContext(ctx).Model(&Mailbox{}).
		Where("owner_id = ?", orgID).Pluck("id", &mailboxIDs)
	var v []emailOut
	if len(mailboxIDs) > 0 {
		var emails []Email
		if err := p.db.WithContext(ctx).Where("mailbox_id IN ?", mailboxIDs).Find(&emails).Error; err != nil {
			return nil, err
		}
		for _, e := range emails {
			v = append(v, emailOut{ID: e.ID, MailboxID: e.MailboxID, From: e.FromAddr,
				To: e.ToAddr, Subject: e.Subject, Read: e.Read, ReceivedAt: e.ReceivedAt})
		}
	}
	return v, nil
}

func jsonResult[T any](v T) (*mcp.CallToolResult, any, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}},
	}, v, nil
}

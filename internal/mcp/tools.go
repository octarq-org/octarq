// Tool handlers for the led MCP server. Each is a read-only projection over the
// core models, scoped to the operator's tenant. They deliberately omit secret
// fields (passwords, raw bodies) — the model never needs them and the roadmap's
// guardrails forbid leaking them.
package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/Jungley8/led/internal/models"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- list_links ---

type listLinksInput struct {
	Host  string `json:"host,omitempty"`  // filter to one serving host (e.g. "go.example.com")
	Tag   string `json:"tag,omitempty"`   // filter to links carrying this tag
	Limit int    `json:"limit,omitempty"` // max links to return (default 50, max 200)
}

type linkOut struct {
	ID        uint      `json:"id"`
	Host      string    `json:"host"`
	Slug      string    `json:"slug"`
	Target    string    `json:"target"`
	Title     string    `json:"title"`
	Tags      string    `json:"tags"`
	Clicks    int64     `json:"clicks"`
	Enabled   bool      `json:"enabled"`
	Archived  bool      `json:"archived"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *server) listLinks(ctx context.Context, _ *mcp.CallToolRequest, in listLinksInput) (*mcp.CallToolResult, []linkOut, error) {
	q := s.gdb.WithContext(ctx).Model(&models.Link{}).
		Where("owner_id = ?", s.ownerScope()).
		Order("clicks DESC").
		Limit(clampLimit(in.Limit, 50))
	if in.Host != "" {
		q = q.Where("host = ?", in.Host)
	}
	var links []models.Link
	if err := q.Find(&links).Error; err != nil {
		return nil, nil, err
	}

	out := make([]linkOut, 0, len(links))
	for _, l := range links {
		if !tagsContain(l.Tags, in.Tag) {
			continue
		}
		out = append(out, linkOut{
			ID: l.ID, Host: l.Host, Slug: l.Slug, Target: l.Target,
			Title: l.Title, Tags: l.Tags, Clicks: l.Clicks,
			Enabled: l.Enabled, Archived: l.Archived, CreatedAt: l.CreatedAt,
		})
	}
	return jsonResult(out)
}

// --- list_mailboxes ---

type listMailboxesInput struct{}

type mailboxOut struct {
	ID      uint   `json:"id"`
	Address string `json:"address"`
	Enabled bool   `json:"enabled"`
	Unread  int64  `json:"unread"`
}

func (s *server) listMailboxes(ctx context.Context, _ *mcp.CallToolRequest, _ listMailboxesInput) (*mcp.CallToolResult, []mailboxOut, error) {
	var mbs []models.Mailbox
	if err := s.gdb.WithContext(ctx).
		Where("owner_id = ?", s.ownerScope()).
		Order("address ASC").Find(&mbs).Error; err != nil {
		return nil, nil, err
	}
	out := make([]mailboxOut, 0, len(mbs))
	for _, mb := range mbs {
		var unread int64
		s.gdb.WithContext(ctx).Model(&models.Email{}).
			Where("mailbox_id = ? AND read = ?", mb.ID, false).Count(&unread)
		out = append(out, mailboxOut{ID: mb.ID, Address: mb.Address, Enabled: mb.Enabled, Unread: unread})
	}
	return jsonResult(out)
}

// --- list_emails ---

type listEmailsInput struct {
	MailboxID  uint `json:"mailboxId,omitempty"` // restrict to one mailbox
	Limit      int  `json:"limit,omitempty"`     // default 30, max 200
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

func (s *server) listEmails(ctx context.Context, _ *mcp.CallToolRequest, in listEmailsInput) (*mcp.CallToolResult, []emailOut, error) {
	// Scope emails to mailboxes the operator owns (emails have no owner_id of
	// their own — ownership is via the mailbox).
	var mailboxIDs []uint
	s.gdb.WithContext(ctx).Model(&models.Mailbox{}).
		Where("owner_id = ?", s.ownerScope()).Pluck("id", &mailboxIDs)
	if len(mailboxIDs) == 0 {
		return jsonResult([]emailOut{})
	}

	q := s.gdb.WithContext(ctx).Model(&models.Email{}).
		Where("mailbox_id IN ?", mailboxIDs).
		Order("received_at DESC").
		Limit(clampLimit(in.Limit, 30))
	if in.MailboxID != 0 {
		q = q.Where("mailbox_id = ?", in.MailboxID)
	}
	if in.UnreadOnly {
		q = q.Where("read = ?", false)
	}
	var emails []models.Email
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

// --- list_domains ---

type listDomainsInput struct{}

type domainOut struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	ForMail bool   `json:"forMail"`
	ForLink bool   `json:"forLink"`
	ZoneID  string `json:"zoneId"`
}

func (s *server) listDomains(ctx context.Context, _ *mcp.CallToolRequest, _ listDomainsInput) (*mcp.CallToolResult, []domainOut, error) {
	var doms []models.Domain
	if err := s.gdb.WithContext(ctx).
		Where("owner_id = ?", s.ownerScope()).
		Order("name ASC").Find(&doms).Error; err != nil {
		return nil, nil, err
	}
	out := make([]domainOut, 0, len(doms))
	for _, d := range doms {
		out = append(out, domainOut{ID: d.ID, Name: d.Name, ForMail: d.ForMail, ForLink: d.ForLink, ZoneID: d.ZoneID})
	}
	return jsonResult(out)
}

// --- query_db_readonly ---

type queryInput struct {
	Query string `json:"query"` // a single read-only SELECT / WITH…SELECT statement
}

type queryOutput struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Count   int              `json:"count"`
	// Truncated is true when the result hit the row cap.
	Truncated bool `json:"truncated"`
}

func (s *server) queryDBReadonly(ctx context.Context, _ *mcp.CallToolRequest, in queryInput) (*mcp.CallToolResult, queryOutput, error) {
	cols, rows, err := s.runReadOnlyQuery(ctx, in.Query)
	if err != nil {
		// Audit the rejected/failed attempt too — the audit trail is exactly
		// where a hallucinated or unsafe query should be visible.
		s.auditQuery(in.Query, 0, err)
		// Return the error as tool content (isError) rather than a transport
		// error, so the model can read and adjust its query.
		msg := fmt.Sprintf("query rejected or failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		}, queryOutput{}, nil
	}
	s.auditQuery(in.Query, len(rows), nil)
	out := queryOutput{Columns: cols, Rows: rows, Count: len(rows), Truncated: len(rows) >= maxRows}
	return jsonResult(out)
}

// --- export_data ---

type exportInput struct {
	Resource string `json:"resource"` // one of: links, emails, domains, mailboxes
}

func (s *server) exportData(ctx context.Context, _ *mcp.CallToolRequest, in exportInput) (*mcp.CallToolResult, any, error) {
	switch in.Resource {
	case "links":
		var v []models.Link
		s.gdb.WithContext(ctx).Where("owner_id = ?", s.ownerScope()).Find(&v)
		return jsonResultAny(v)
	case "domains":
		var v []models.Domain
		s.gdb.WithContext(ctx).Where("owner_id = ?", s.ownerScope()).Find(&v)
		return jsonResultAny(v)
	case "mailboxes":
		var v []models.Mailbox
		s.gdb.WithContext(ctx).Where("owner_id = ?", s.ownerScope()).Find(&v)
		return jsonResultAny(v)
	case "emails":
		// Project away secret/bulky columns (raw, html) for a portable export.
		var mailboxIDs []uint
		s.gdb.WithContext(ctx).Model(&models.Mailbox{}).
			Where("owner_id = ?", s.ownerScope()).Pluck("id", &mailboxIDs)
		var v []emailOut
		if len(mailboxIDs) > 0 {
			var emails []models.Email
			s.gdb.WithContext(ctx).Where("mailbox_id IN ?", mailboxIDs).Find(&emails)
			for _, e := range emails {
				v = append(v, emailOut{ID: e.ID, MailboxID: e.MailboxID, From: e.FromAddr,
					To: e.ToAddr, Subject: e.Subject, Read: e.Read, ReceivedAt: e.ReceivedAt})
			}
		}
		return jsonResultAny(v)
	default:
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "unknown resource; use one of: links, emails, domains, mailboxes"}},
		}, nil, nil
	}
}

// jsonResultAny is jsonResult for the `any`-typed export handler.
func jsonResultAny(v any) (*mcp.CallToolResult, any, error) {
	r, _, err := jsonResult(v)
	return r, v, err
}

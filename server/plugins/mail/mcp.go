package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

// errNoOrgInContext refuses a tool call that arrives without a tenant scope.
// Every transport supplies one — the networked ones from the caller's API token,
// the stdio CLI from its entry point — so an absent org means something is wrong,
// and defaulting it to a tenant would hand that tenant's data to whoever asked.
var errNoOrgInContext = errors.New("no workspace in this request")

type listMailboxesInput struct{}

type mailboxOut struct {
	ID      uint   `json:"id"`
	Address string `json:"address"`
	Enabled bool   `json:"enabled"`
	Unread  int64  `json:"unread"`
}

type listEmailsInput struct {
	MailboxID       uint `json:"mailboxId,omitempty" jsonschema:"Optional mailbox ID to filter emails"`
	MailboxIDSnake  uint `json:"mailbox_id,omitempty" jsonschema:"Alternative snake_case mailbox ID"`
	Limit           int  `json:"limit,omitempty" jsonschema:"Maximum number of emails to return (default 30, max 200)"`
	UnreadOnly      bool `json:"unreadOnly,omitempty" jsonschema:"Only return unread emails if true"`
	UnreadOnlySnake bool `json:"unread_only,omitempty" jsonschema:"Alternative snake_case unread_only filter"`
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

type getLatestOTPInput struct {
	MailboxID        uint   `json:"mailbox_id,omitempty" jsonschema:"Optional mailbox ID to filter unread emails"`
	MailboxAddress   string `json:"mailbox_address,omitempty" jsonschema:"Optional mailbox address (e.g. user@example.com) to filter unread emails"`
	MailboxIDCamel   uint   `json:"mailboxId,omitempty" jsonschema:"Alternative camelCase mailbox ID"`
	MailboxAddrCamel string `json:"mailboxAddress,omitempty" jsonschema:"Alternative camelCase mailbox address"`
}

type otpResult struct {
	Found      bool      `json:"found"`
	OTP        string    `json:"otp,omitempty"`
	EmailID    uint      `json:"email_id,omitempty"`
	MailboxID  uint      `json:"mailbox_id,omitempty"`
	Subject    string    `json:"subject,omitempty"`
	From       string    `json:"from,omitempty"`
	To         string    `json:"to,omitempty"`
	ReceivedAt time.Time `json:"received_at,omitempty"`
	Message    string    `json:"message,omitempty"`
}

type getEmailSummaryInput struct {
	EmailID      uint `json:"email_id,omitempty" jsonschema:"The unique ID of the email to summarize"`
	EmailIDCamel uint `json:"emailId,omitempty" jsonschema:"Alternative camelCase email ID"`
}

type emailSummaryOut struct {
	ID         uint      `json:"id"`
	MailboxID  uint      `json:"mailbox_id"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Subject    string    `json:"subject"`
	Summary    string    `json:"summary"`
	Category   string    `json:"category"`
	OTP        string    `json:"otp,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
}

type getEmailContentInput struct {
	EmailID      uint `json:"email_id,omitempty" jsonschema:"The unique ID of the email to fetch sanitized content for"`
	EmailIDCamel uint `json:"emailId,omitempty" jsonschema:"Alternative camelCase email ID"`
}

type emailContentOut struct {
	ID         uint      `json:"id"`
	MailboxID  uint      `json:"mailbox_id"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Subject    string    `json:"subject"`
	Text       string    `json:"text"`
	Truncated  bool      `json:"truncated"`
	ReceivedAt time.Time `json:"received_at"`
}

func (p *Plugin) RegisterMCP(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_mailboxes",
		Description: "List email mailboxes with their unread counts.",
	}, p.mcpListMailboxes)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_emails",
		Description: "List recently received emails (subject, from, to, date) — optionally for one mailbox. Bodies are not returned; open the full message in the dashboard.",
	}, p.mcpListEmails)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_latest_otp",
		Description: "Get the latest one-time verification code (OTP) from unread emails received in the last 10 minutes, optionally filtered by mailbox ID or address.",
	}, p.mcpGetLatestOTP)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_email_summary",
		Description: "Get a structured, sanitized summary of an email by ID, including detected category and any extracted verification code (OTP).",
	}, p.mcpGetEmailSummary)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_email_content",
		Description: "Get the sanitized plain text content of an email by ID with safety guardrails (sensitive tokens redacted, inline base64 images stripped, capped at 4KB).",
	}, p.mcpGetEmailContent)
}

func (p *Plugin) mcpListMailboxes(ctx context.Context, _ *mcp.CallToolRequest, _ listMailboxesInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
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
		return nil, nil, errNoOrgInContext
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 30
	} else if limit > 200 {
		limit = 200
	}
	mbID := in.MailboxID
	if mbID == 0 && in.MailboxIDSnake != 0 {
		mbID = in.MailboxIDSnake
	}
	unreadOnly := in.UnreadOnly || in.UnreadOnlySnake

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
	if mbID != 0 {
		q = q.Where("mailbox_id = ?", mbID)
	}
	if unreadOnly {
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

func (p *Plugin) mcpGetLatestOTP(ctx context.Context, _ *mcp.CallToolRequest, in getLatestOTPInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}

	mbID := in.MailboxID
	if mbID == 0 && in.MailboxIDCamel != 0 {
		mbID = in.MailboxIDCamel
	}
	mbAddr := strings.TrimSpace(in.MailboxAddress)
	if mbAddr == "" && in.MailboxAddrCamel != "" {
		mbAddr = strings.TrimSpace(in.MailboxAddrCamel)
	}

	var mailboxIDs []uint
	q := p.db.WithContext(ctx).Model(&Mailbox{}).Where("owner_id = ?", orgID)
	if mbID != 0 {
		q = q.Where("id = ?", mbID)
	}
	if mbAddr != "" {
		q = q.Where("LOWER(address) = LOWER(?)", mbAddr)
	}
	if err := q.Pluck("id", &mailboxIDs).Error; err != nil {
		return nil, nil, err
	}
	if len(mailboxIDs) == 0 {
		return jsonResult(otpResult{
			Found:   false,
			Message: "no matching mailbox found in workspace",
		})
	}

	tenMinutesAgo := time.Now().Add(-10 * time.Minute)
	var emails []Email
	if err := p.db.WithContext(ctx).Model(&Email{}).
		Where("mailbox_id IN ? AND read = ? AND received_at >= ?", mailboxIDs, false, tenMinutesAgo).
		Order("received_at DESC").
		Limit(20).
		Find(&emails).Error; err != nil {
		return nil, nil, err
	}

	for _, e := range emails {
		otp := ExtractOTP(e.Subject, e.Text, e.HTML)
		if otp != "" {
			return jsonResult(otpResult{
				Found:      true,
				OTP:        otp,
				EmailID:    e.ID,
				MailboxID:  e.MailboxID,
				Subject:    e.Subject,
				From:       e.FromAddr,
				To:         e.ToAddr,
				ReceivedAt: e.ReceivedAt,
			})
		}
	}

	return jsonResult(otpResult{
		Found:   false,
		Message: "no unread OTP email found within the last 10 minutes",
	})
}

func (p *Plugin) mcpGetEmailSummary(ctx context.Context, _ *mcp.CallToolRequest, in getEmailSummaryInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	emailID := in.EmailID
	if emailID == 0 && in.EmailIDCamel != 0 {
		emailID = in.EmailIDCamel
	}
	email, err := p.getEmailForOrg(ctx, emailID, orgID)
	if err != nil {
		return nil, nil, err
	}

	otp := ExtractOTP(email.Subject, email.Text, email.HTML)
	body := email.Text
	if body == "" && email.HTML != "" {
		body = stripHTML(email.HTML)
	}
	summary, category := GenerateEmailSummary(email.Subject, body, otp)

	return jsonResult(emailSummaryOut{
		ID:         email.ID,
		MailboxID:  email.MailboxID,
		From:       email.FromAddr,
		To:         email.ToAddr,
		Subject:    email.Subject,
		Summary:    summary,
		Category:   category,
		OTP:        otp,
		ReceivedAt: email.ReceivedAt,
	})
}

func (p *Plugin) mcpGetEmailContent(ctx context.Context, _ *mcp.CallToolRequest, in getEmailContentInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	emailID := in.EmailID
	if emailID == 0 && in.EmailIDCamel != 0 {
		emailID = in.EmailIDCamel
	}
	email, err := p.getEmailForOrg(ctx, emailID, orgID)
	if err != nil {
		return nil, nil, err
	}

	body := email.Text
	if body == "" && email.HTML != "" {
		body = stripHTML(email.HTML)
	}

	sanitizedText, truncated := SanitizeEmailContent(body)

	return jsonResult(emailContentOut{
		ID:         email.ID,
		MailboxID:  email.MailboxID,
		From:       email.FromAddr,
		To:         email.ToAddr,
		Subject:    email.Subject,
		Text:       sanitizedText,
		Truncated:  truncated,
		ReceivedAt: email.ReceivedAt,
	})
}

func (p *Plugin) getEmailForOrg(ctx context.Context, emailID uint, orgID uint) (*Email, error) {
	if emailID == 0 {
		return nil, plugin.NewAgentError(400, "BAD_REQUEST", "email_id is required", "Provide a valid email_id", false)
	}
	var email Email
	err := p.db.WithContext(ctx).
		Joins("JOIN mailboxes ON mailboxes.id = emails.mailbox_id").
		Where("emails.id = ? AND mailboxes.owner_id = ?", emailID, orgID).
		First(&email).Error
	if err != nil {
		return nil, plugin.NewAgentError(404, "EMAIL_NOT_FOUND", "email not found in workspace", "Verify that the email ID is correct and belongs to your workspace", false)
	}
	return &email, nil
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

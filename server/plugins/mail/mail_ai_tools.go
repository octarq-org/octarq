package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/mail"
	"github.com/octarq-org/octarq/server/internal/usagemetric"
	"github.com/octarq-org/octarq/server/plugin"
)

// AttachmentDTO carries metadata of an email attachment.
type AttachmentDTO struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	Rejected    string `json:"rejected,omitempty"`
}

// ReadEmailInput is the argument contract for mail_get / read_email_body.
type ReadEmailInput struct {
	ID           uint `path:"id" query:"id" json:"id,omitempty" jsonschema:"The unique ID of the email to read"`
	EmailID      uint `json:"email_id,omitempty" jsonschema:"Alternative snake_case email ID"`
	EmailIDCamel uint `json:"emailId,omitempty" jsonschema:"Alternative camelCase email ID"`
}

func (in ReadEmailInput) resolveID() uint {
	if in.ID != 0 {
		return in.ID
	}
	if in.EmailID != 0 {
		return in.EmailID
	}
	return in.EmailIDCamel
}

// ReadEmailOutput carries email details including plain text inside untrusted fences.
type ReadEmailOutput struct {
	ID          uint            `json:"id"`
	MessageID   string          `json:"messageId,omitempty"`
	MailboxID   uint            `json:"mailboxId"`
	From        string          `json:"from"`
	To          []string        `json:"to"`
	Subject     string          `json:"subject"`
	ReceivedAt  time.Time       `json:"receivedAt"`
	TextBody    string          `json:"textBody"`
	HTMLPresent bool            `json:"htmlPresent"`
	Attachments []AttachmentDTO `json:"attachments"`
	Guard       string          `json:"guard,omitempty"`
}

// CreateDraftInput is the argument contract for mail_draft_create / create_email_draft.
type CreateDraftInput struct {
	MailboxID      uint     `json:"mailbox_id,omitempty" jsonschema:"Mailbox ID to draft from"`
	MailboxIDCamel uint     `json:"mailboxId,omitempty" jsonschema:"Alternative camelCase mailbox ID"`
	To             string   `json:"to,omitempty" jsonschema:"Recipient email address or comma-separated addresses"`
	ToList         []string `json:"toList,omitempty" jsonschema:"Optional list of recipient addresses"`
	CC             string   `json:"cc,omitempty" jsonschema:"Optional CC email address(es)"`
	CCList         []string `json:"ccList,omitempty" jsonschema:"Optional list of CC addresses"`
	Subject        string   `json:"subject" jsonschema:"Draft email subject line"`
	Body           string   `json:"body" jsonschema:"Draft email body text"`
	InReplyToID    uint     `json:"in_reply_to_id,omitempty" jsonschema:"Optional email ID this draft is replying to"`
	InReplyToCamel uint     `json:"inReplyToId,omitempty" jsonschema:"Alternative camelCase in_reply_to_id"`
}

// CreateDraftOutput reports the saved draft record.
type CreateDraftOutput struct {
	ID          uint      `json:"id"`
	MailboxID   uint      `json:"mailboxId,omitempty"`
	To          string    `json:"to"`
	CC          string    `json:"cc,omitempty"`
	Subject     string    `json:"subject"`
	Body        string    `json:"body"`
	InReplyToID uint      `json:"inReplyToId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// SendEmailToolInput is the argument contract for mail_send_reply / send_email.
type SendEmailToolInput struct {
	SMTPSenderID uint     `json:"smtp_sender_id,omitempty" jsonschema:"Optional SMTP sender ID to use"`
	To           []string `json:"to,omitempty" jsonschema:"List of recipient email addresses"`
	ToAddr       string   `json:"to_addr,omitempty" jsonschema:"Single recipient or comma-separated addresses"`
	CC           []string `json:"cc,omitempty" jsonschema:"Optional list of CC addresses"`
	Subject      string   `json:"subject" jsonschema:"Subject of the email"`
	Body         string   `json:"body" jsonschema:"Plain text body of the email"`
	InReplyToID  uint     `json:"in_reply_to_id,omitempty" jsonschema:"Optional email ID being replied to"`
}

// SendEmailToolOutput reports result of the outbound email delivery.
type SendEmailToolOutput struct {
	Success bool   `json:"success"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

func (p *Plugin) handleReadEmailBody(ctx context.Context, in ReadEmailInput) (*ReadEmailOutput, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, plugin.NewAgentError(401, "UNAUTHORIZED", "no workspace in this request", "Provide a valid workspace context.", false)
	}
	emailID := in.resolveID()
	if emailID == 0 {
		return nil, plugin.NewAgentError(400, "BAD_REQUEST", "email id is required", "Provide a valid email ID to read.", false)
	}

	email, err := p.getEmailForOrg(ctx, emailID, orgID)
	if err != nil {
		return nil, err
	}

	rawBody := email.Text
	if rawBody == "" && email.HTML != "" {
		rawBody = stripHTML(email.HTML)
	}

	cleanText := StripInvisibleControls(rawBody)
	cleanText = NeutralizeFramingTags(cleanText)
	masked, _ := SanitizeEmailContent(cleanText)

	fencedBody := fmt.Sprintf("<untrusted>\n%s\n</untrusted>", masked)

	var atts []AttachmentDTO
	if email.Attachments != "" {
		var rawAtts []struct {
			Filename    string `json:"filename"`
			ContentType string `json:"contentType"`
			Size        int64  `json:"size"`
			Rejected    string `json:"rejected,omitempty"`
		}
		if err := json.Unmarshal([]byte(email.Attachments), &rawAtts); err == nil {
			atts = make([]AttachmentDTO, 0, len(rawAtts))
			for _, a := range rawAtts {
				atts = append(atts, AttachmentDTO{
					Filename:    a.Filename,
					ContentType: a.ContentType,
					Size:        a.Size,
					Rejected:    a.Rejected,
				})
			}
		}
	}

	var toAddrs []string
	if email.ToAddr != "" {
		for _, part := range strings.Split(email.ToAddr, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				toAddrs = append(toAddrs, sanitizeMailMeta(trimmed))
			}
		}
	}

	return &ReadEmailOutput{
		ID:          email.ID,
		MessageID:   email.MessageID,
		MailboxID:   email.MailboxID,
		From:        sanitizeMailMeta(email.FromAddr),
		To:          toAddrs,
		Subject:     sanitizeMailMeta(email.Subject),
		ReceivedAt:  email.ReceivedAt,
		TextBody:    fencedBody,
		HTMLPresent: strings.TrimSpace(email.HTML) != "",
		Attachments: atts,
		Guard:       AgentBodyGuard,
	}, nil
}

func (p *Plugin) handleCreateDraft(ctx context.Context, in CreateDraftInput) (*CreateDraftOutput, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, plugin.NewAgentError(401, "UNAUTHORIZED", "no workspace in this request", "Provide a valid workspace context.", false)
	}

	to := strings.TrimSpace(in.To)
	if to == "" && len(in.ToList) > 0 {
		to = strings.Join(in.ToList, ", ")
	}
	if to == "" {
		return nil, plugin.NewAgentError(400, "BAD_REQUEST", "recipient email address (to) is required", "Specify a recipient address for the draft.", false)
	}

	cc := strings.TrimSpace(in.CC)
	if cc == "" && len(in.CCList) > 0 {
		cc = strings.Join(in.CCList, ", ")
	}

	mbID := in.MailboxID
	if mbID == 0 && in.MailboxIDCamel != 0 {
		mbID = in.MailboxIDCamel
	}
	tdb := p.tenantDB(orgID)
	if mbID != 0 && tdb != nil {
		var count int64
		tdb.Model(&Mailbox{}).Where("id = ?", mbID).Count(&count)
		if count == 0 {
			return nil, plugin.NewAgentError(400, "MAILBOX_NOT_FOUND", "specified mailbox not found in workspace", "Select an existing mailbox belonging to your workspace.", false)
		}
	} else if tdb != nil {
		var mb Mailbox
		if err := tdb.First(&mb).Error; err == nil {
			mbID = mb.ID
		}
	}

	replyToID := in.InReplyToID
	if replyToID == 0 && in.InReplyToCamel != 0 {
		replyToID = in.InReplyToCamel
	}

	now := time.Now()
	draft := MailDraft{
		OrgID:       orgID,
		MailboxID:   mbID,
		To:          to,
		CC:          cc,
		Subject:     in.Subject,
		Body:        in.Body,
		InReplyToID: replyToID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := p.db.WithContext(ctx).Create(&draft).Error; err != nil {
		return nil, plugin.NewAgentError(500, "DATABASE_ERROR", "failed to save draft", "Try saving the draft again later.", false)
	}

	return &CreateDraftOutput{
		ID:          draft.ID,
		MailboxID:   draft.MailboxID,
		To:          draft.To,
		CC:          draft.CC,
		Subject:     draft.Subject,
		Body:        draft.Body,
		InReplyToID: draft.InReplyToID,
		CreatedAt:   draft.CreatedAt,
	}, nil
}

func (p *Plugin) handleSendEmail(ctx context.Context, in SendEmailToolInput) (*SendEmailToolOutput, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, plugin.NewAgentError(401, "UNAUTHORIZED", "no workspace in this request", "Provide a valid workspace context.", false)
	}

	var toAddrs []string
	if len(in.To) > 0 {
		for _, addr := range in.To {
			trimmed := strings.TrimSpace(addr)
			if trimmed != "" {
				toAddrs = append(toAddrs, trimmed)
			}
		}
	} else if in.ToAddr != "" {
		for _, addr := range strings.FieldsFunc(in.ToAddr, func(r rune) bool {
			return r == ',' || r == ';'
		}) {
			trimmed := strings.TrimSpace(addr)
			if trimmed != "" {
				toAddrs = append(toAddrs, trimmed)
			}
		}
	}

	if len(toAddrs) == 0 {
		return nil, plugin.NewAgentError(400, "BAD_REQUEST", "recipient email address (to) is required", "Provide at least one recipient address.", false)
	}

	suppressedAddr, err := p.anySuppressed(orgID, toAddrs)
	if err != nil {
		return nil, plugin.NewAgentError(500, "SUPPRESSION_CHECK_FAILED", "could not check suppression list", "Try again later.", false)
	}
	if suppressedAddr != "" {
		return nil, plugin.NewAgentError(400, "RECIPIENT_SUPPRESSED", fmt.Sprintf("recipient %s is on the suppression list", suppressedAddr), "Remove the recipient from suppressions or use another address.", false)
	}

	orgKey := fmt.Sprintf("org:%d", orgID)
	if p.sendLimiter != nil && !p.sendLimiter.allow(orgKey) {
		return nil, plugin.NewAgentError(429, "RATE_LIMIT_EXCEEDED", "outbound send rate limit exceeded", "Wait before sending more emails.", false)
	}

	var senderConfig SMTPSender
	q := p.db.WithContext(ctx).Where("owner_id = ?", orgID)
	if in.SMTPSenderID != 0 {
		q = q.Where("id = ?", in.SMTPSenderID)
	} else {
		q = q.Order("id ASC")
	}
	if err := q.First(&senderConfig).Error; err != nil {
		return nil, plugin.NewAgentError(400, "SMTP_SENDER_NOT_FOUND", "no configured SMTP sender found for this workspace", "Configure an SMTP sender in workspace settings.", false)
	}

	if p.decrypt == nil {
		return nil, plugin.NewAgentError(500, "DECRYPTION_UNAVAILABLE", "decryption service unavailable", "Verify encryption configuration.", false)
	}
	pass, err := p.decrypt(senderConfig.Pass)
	if err != nil {
		return nil, plugin.NewAgentError(500, "DECRYPTION_FAILED", "could not decrypt SMTP sender credentials", "Update sender credentials.", false)
	}

	if err := plugin.CheckQuota(p.ctx, ctx, orgID, "mailOutPerMonth", int64(len(toAddrs))); err != nil {
		if errors.Is(err, plugin.ErrQuotaUnavailable) {
			return nil, plugin.NewAgentError(402, "PAYMENT_REQUIRED", "outbound mail is not included in this plan", "Upgrade your plan to unlock email sending.", false)
		}
		return nil, plugin.NewAgentError(429, "QUOTA_EXCEEDED", "outbound mail quota exceeded for workspace", "Upgrade your plan or wait for quota reset.", false)
	}

	allRecipients := append([]string(nil), toAddrs...)
	for _, cc := range in.CC {
		trimmed := strings.TrimSpace(cc)
		if trimmed != "" {
			allRecipients = append(allRecipients, trimmed)
		}
	}

	subject := in.Subject
	if in.InReplyToID != 0 && !strings.HasPrefix(strings.ToLower(subject), "re:") {
		var orig Email
		if err := p.db.WithContext(ctx).Where("id = ?", in.InReplyToID).First(&orig).Error; err == nil && orig.Subject != "" {
			if subject == "" {
				subject = "Re: " + orig.Subject
			}
		}
	}

	msg := mail.Message{
		From:    senderConfig.FromEmail,
		To:      allRecipients,
		Subject: subject,
		Text:    in.Body,
	}

	sender := mail.NewCustomSender(senderConfig.Host, fmt.Sprint(senderConfig.Port), senderConfig.User, string(pass), senderConfig.FromEmail)
	if err := sender.Send(msg); err != nil {
		if p.publishEvent != nil {
			p.publishEvent(orgID, "email.send_failed", map[string]any{"to": msg.To, "subject": msg.Subject, "error": err.Error()})
		}
		return nil, plugin.NewAgentError(400, "SEND_FAILED", fmt.Sprintf("could not deliver email: %v", err), "Check SMTP settings and recipient address.", false)
	}

	if p.recordUsage != nil {
		p.recordUsage(orgID, usagemetric.MailOut, int64(len(msg.To)))
	}
	p.upsertContacts(orgID, toAddrs)
	p.recordSentEmail(orgID, senderConfig.FromEmail, strings.Join(toAddrs, ", "), msg.Subject, msg.Text, msg.HTML)
	if p.sendLimiter != nil {
		p.sendLimiter.recordFailure(orgKey)
	}

	return &SendEmailToolOutput{
		Success: true,
		To:      strings.Join(toAddrs, ", "),
		Subject: in.Subject,
		Message: "Email sent successfully",
	}, nil
}

func (p *Plugin) registerDeclarativeEndpoints(ctx *plugin.Context) {
	// 1. mail_get / read_email_body (RiskLevelRead)
	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[ReadEmailInput, ReadEmailOutput]{
		Name:        "read_email_body",
		MCPToolName: "mail_get",
		Method:      "GET",
		Path:        "/api/mail/emails/{id}/body",
		Summary:     "Read Email Body",
		Description: "Read one email by ID including its sanitized plain-text body in <untrusted> fences and attachment metadata list.",
		RiskLevel:   plugin.RiskLevelRead,
		RequireAuth: true,
		ExposeMCP:   true,
		Handler:     p.handleReadEmailBody,
	})

	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[ReadEmailInput, ReadEmailOutput]{
		Name:        "mail_get",
		MCPToolName: "octarq_network__mail_get",
		Method:      "POST",
		Path:        "/api/mail/ai/get",
		Summary:     "Get Email Body",
		Description: "Read one email by ID including its sanitized plain-text body in <untrusted> fences and attachment metadata list.",
		RiskLevel:   plugin.RiskLevelRead,
		RequireAuth: true,
		ExposeMCP:   true,
		Handler:     p.handleReadEmailBody,
	})

	// 2. mail_draft_create / create_email_draft (RiskLevelWrite)
	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[CreateDraftInput, CreateDraftOutput]{
		Name:        "create_email_draft",
		MCPToolName: "mail_draft_create",
		Method:      "POST",
		Path:        "/api/mail/drafts/create",
		Summary:     "Create Email Draft",
		Description: "Create and persist a draft email with recipients, subject, and body in mail_drafts.",
		RiskLevel:   plugin.RiskLevelWrite,
		RequireAuth: true,
		ExposeMCP:   true,
		Handler:     p.handleCreateDraft,
	})

	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[CreateDraftInput, CreateDraftOutput]{
		Name:        "mail_draft_create",
		MCPToolName: "octarq_network__mail_draft_create",
		Method:      "POST",
		Path:        "/api/mail/ai/draft",
		Summary:     "Create Email Draft",
		Description: "Create and persist a draft email with recipients, subject, and body in mail_drafts.",
		RiskLevel:   plugin.RiskLevelWrite,
		RequireAuth: true,
		ExposeMCP:   true,
		Handler:     p.handleCreateDraft,
	})

	// 3. mail_send_reply / send_email (RiskLevelDestructive, RequireApproval: true)
	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[SendEmailToolInput, SendEmailToolOutput]{
		Name:            "send_email",
		MCPToolName:     "mail_send_reply",
		Method:          "POST",
		Path:            "/api/mail/emails/send-reply",
		Summary:         "Send Email or Reply",
		Description:     "Send an outbound email or reply via the configured SMTP relay. Requires HITL approval with recipient, subject, and body preview.",
		RiskLevel:       plugin.RiskLevelDestructive,
		RequireApproval: true,
		RequireAuth:     true,
		ExposeMCP:       true,
		Handler:         p.handleSendEmail,
	})

	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[SendEmailToolInput, SendEmailToolOutput]{
		Name:            "mail_send_reply",
		MCPToolName:     "octarq_network__mail_send_reply",
		Method:          "POST",
		Path:            "/api/mail/ai/send",
		Summary:         "Send Email or Reply",
		Description:     "Send an outbound email or reply via the configured SMTP relay. Requires HITL approval with recipient, subject, and body preview.",
		RiskLevel:       plugin.RiskLevelDestructive,
		RequireApproval: true,
		RequireAuth:     true,
		ExposeMCP:       true,
		Handler:         p.handleSendEmail,
	})
}

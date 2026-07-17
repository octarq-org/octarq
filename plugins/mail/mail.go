package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
)

// --- mailboxes ---

type ListMailboxesInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListMailboxesInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListMailboxesOutput struct {
	Body []models.Mailbox
}

func (p *Plugin) listMailboxes(ctx context.Context, input *ListMailboxesInput) (*ListMailboxesOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var boxes []models.Mailbox
	p.orgDB(r).Order("created_at DESC").Find(&boxes)
	// Attach unread counts.
	for i := range boxes {
		p.db.Model(&models.Email{}).
			Where("mailbox_id = ? AND read = ?", boxes[i].ID, false).
			Count(&boxes[i].Unread)
	}
	return &ListMailboxesOutput{Body: boxes}, nil
}

type mailboxDTO struct {
	Address string `json:"address,omitempty"`
	Note    string `json:"note,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type CreateMailboxInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body mailboxDTO
}

func (i *CreateMailboxInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateMailboxOutput struct {
	Body models.Mailbox
}

func (p *Plugin) createMailbox(ctx context.Context, input *CreateMailboxInput) (*CreateMailboxOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	addr := strings.TrimSpace(strings.ToLower(input.Body.Address))
	if !strings.Contains(addr, "@") {
		return nil, huma.Error400BadRequest("address must be a full email")
	}
	enabled := true
	if input.Body.Enabled != nil {
		enabled = *input.Body.Enabled
	}
	mb := models.Mailbox{
		OrgID:   p.orgID(r),
		Address: addr, Note: input.Body.Note, Enabled: enabled,
	}
	if err := p.db.Create(&mb).Error; err != nil {
		return nil, huma.NewError(http.StatusConflict, "mailbox already exists")
	}
	p.audit(r, "mailbox.create", "mailbox", mb.ID, map[string]any{"address": mb.Address})
	return &CreateMailboxOutput{Body: mb}, nil
}

type UpdateMailboxInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body mailboxDTO
}

func (i *UpdateMailboxInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateMailboxOutput struct {
	Body models.Mailbox
}

func (p *Plugin) updateMailbox(ctx context.Context, input *UpdateMailboxInput) (*UpdateMailboxOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var mb models.Mailbox
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&mb).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	mb.Note = input.Body.Note
	if input.Body.Enabled != nil {
		mb.Enabled = *input.Body.Enabled
	}
	p.db.Save(&mb)
	meta := map[string]any{
		"note":    mb.Note,
		"enabled": mb.Enabled,
	}
	p.audit(r, "mailbox.update", "mailbox", mb.ID, meta)
	return &UpdateMailboxOutput{Body: mb}, nil
}

type DeleteMailboxInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteMailboxInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteMailboxOutput struct {
	Body map[string]bool
}

func (p *Plugin) deleteMailbox(ctx context.Context, input *DeleteMailboxInput) (*DeleteMailboxOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	res := p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).Delete(&models.Mailbox{})
	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	p.db.Where("mailbox_id = ?", input.ID).Delete(&models.Email{})
	p.audit(r, "mailbox.delete", "mailbox", input.ID, nil)
	return &DeleteMailboxOutput{Body: map[string]bool{"ok": true}}, nil
}

// --- emails ---

type ListEmailsInput struct {
	Ctx     huma.Context `hidden:"true"`
	Mailbox string       `query:"mailbox"`
	Q       string       `query:"q"`
	Limit   int          `query:"limit"`
	Offset  int          `query:"offset"`
}

func (i *ListEmailsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListEmailsOutput struct {
	Body []models.Email
}

func (p *Plugin) listEmails(ctx context.Context, input *ListEmailsInput) (*ListEmailsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgMailboxes := p.db.Model(&models.Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	q := p.db.Where("mailbox_id IN (?)", orgMailboxes).Order("received_at DESC").Omit("Raw", "HTML")
	if input.Mailbox != "" {
		q = q.Where("mailbox_id = ?", input.Mailbox)
	}
	if input.Q != "" {
		like := "%" + input.Q + "%"
		q = q.Where("subject LIKE ? OR from_addr LIKE ? OR text LIKE ? OR note LIKE ?", like, like, like, like)
	}
	limit := 50
	if input.Limit > 0 && input.Limit <= 500 {
		limit = input.Limit
	}
	offset := 0
	if input.Offset > 0 {
		offset = input.Offset
	}
	q = q.Limit(limit).Offset(offset)
	var emails []models.Email
	q.Find(&emails)
	return &ListEmailsOutput{Body: emails}, nil
}

type GetEmailInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *GetEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetEmailOutput struct {
	Body models.Email
}

func (p *Plugin) getEmail(ctx context.Context, input *GetEmailInput) (*GetEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.emailBelongsToOrg(input.ID, p.orgID(r)) {
		return nil, huma.Error404NotFound("not found")
	}
	var e models.Email
	if p.db.First(&e, input.ID).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	if !e.Read {
		p.db.Model(&e).Update("read", true)
		e.Read = true
	}
	return &GetEmailOutput{Body: e}, nil
}

type UpdateEmailInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body struct {
		Read *bool   `json:"read,omitempty"`
		Note *string `json:"note,omitempty"`
	}
}

func (i *UpdateEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateEmailOutput struct {
	Body models.Email
}

func (p *Plugin) updateEmail(ctx context.Context, input *UpdateEmailInput) (*UpdateEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.emailBelongsToOrg(input.ID, p.orgID(r)) {
		return nil, huma.Error404NotFound("not found")
	}
	var e models.Email
	if p.db.First(&e, input.ID).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	if input.Body.Read != nil {
		e.Read = *input.Body.Read
	}
	if input.Body.Note != nil {
		e.Note = *input.Body.Note
	}
	p.db.Save(&e)
	return &UpdateEmailOutput{Body: e}, nil
}

type ReadAllEmailsInput struct {
	Ctx     huma.Context `hidden:"true"`
	Mailbox string       `query:"mailbox"`
}

func (i *ReadAllEmailsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ReadAllEmailsOutput struct {
	Body map[string]any
}

// readAllEmails marks every email read, optionally scoped to one mailbox.
func (p *Plugin) readAllEmails(ctx context.Context, input *ReadAllEmailsInput) (*ReadAllEmailsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgMailboxes := p.db.Model(&models.Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	q := p.db.Model(&models.Email{}).Where("read = ? AND mailbox_id IN (?)", false, orgMailboxes)
	if input.Mailbox != "" {
		q = q.Where("mailbox_id = ?", input.Mailbox)
	}
	res := q.Update("read", true)
	return &ReadAllEmailsOutput{
		Body: map[string]any{"ok": true, "updated": res.RowsAffected},
	}, nil
}

type RawEmailInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *RawEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// rawEmail streams the original RFC822 message as a downloadable .eml file.
func (p *Plugin) rawEmail(ctx context.Context, input *RawEmailInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.emailBelongsToOrg(input.ID, p.orgID(r)) {
		return nil, huma.Error404NotFound("not found")
	}
	var e models.Email
	if p.db.First(&e, input.ID).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	w.Header().Set("Content-Type", "message/rfc822")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"email-%d.eml\"", e.ID))
	w.Write(e.Raw)
	return nil, nil
}

type DeleteEmailInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteEmailOutput struct {
	Body map[string]bool
}

func (p *Plugin) deleteEmail(ctx context.Context, input *DeleteEmailInput) (*DeleteEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.emailBelongsToOrg(input.ID, p.orgID(r)) {
		return nil, huma.Error404NotFound("not found")
	}
	p.db.Delete(&models.Email{}, input.ID)
	return &DeleteEmailOutput{Body: map[string]bool{"ok": true}}, nil
}

type SendEmailInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		mail.Message
		SMTPSenderID uint `json:"smtpSenderId"`
		TrackLinks   bool `json:"trackLinks"`
	}
}

func (i *SendEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type SendEmailOutput struct {
	Body map[string]bool
}

func (p *Plugin) sendEmail(ctx context.Context, input *SendEmailInput) (*SendEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if len(input.Body.To) == 0 {
		return nil, huma.Error400BadRequest("to is required")
	}

	// Rate-limit outbound mail per org so a leaked API token or a runaway client
	// can't burn the sending domain's / relay IP's reputation into an RBL.
	orgKey := fmt.Sprintf("org:%d", p.orgID(r))
	if !p.sendLimiter.allow(orgKey) {
		return nil, huma.Error429TooManyRequests("send rate limit exceeded (max 100/hour) — try again later")
	}

	if input.Body.SMTPSenderID == 0 {
		return nil, huma.Error400BadRequest("no SMTP sender selected")
	}
	var s models.SMTPSender
	if err := p.db.Where("id = ? AND owner_id = ?", input.Body.SMTPSenderID, p.orgID(r)).First(&s).Error; err != nil {
		return nil, huma.Error400BadRequest("invalid smtp sender id")
	}
	pass, err := p.decrypt(s.Pass)
	if err != nil {
		return nil, huma.Error500InternalServerError("decrypt failed")
	}
	// Force the From header to the sender's verified address — never trust a
	// client-supplied From, which would let a caller spoof arbitrary senders
	// through the relay.
	msg := input.Body.Message
	msg.From = s.FromEmail
	sender := mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)

	if input.Body.TrackLinks {
		p.wrapLinksInEmail(r, &msg)
	}

	if err := sender.Send(msg); err != nil {
		return nil, huma.Error400BadRequest("send failed: " + err.Error())
	}
	p.sendLimiter.recordFailure(orgKey) // count this send against the per-org cap
	return &SendEmailOutput{Body: map[string]bool{"ok": true}}, nil
}

func (p *Plugin) wrapLinksInEmail(r *http.Request, msg *mail.Message) {
	orgID := p.orgID(r)

	// Determine the short link domain host
	var doms []models.Domain
	p.db.Where("owner_id = ? AND for_link = ?", orgID, true).Find(&doms)
	shortHost := r.Host
	if len(doms) > 0 {
		shortHost = doms[0].Name
	}
	// Strip port from host
	if idx := strings.IndexByte(shortHost, ':'); idx >= 0 {
		shortHost = shortHost[:idx]
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	// Regex for HTTP/HTTPS links
	reLink := regexp.MustCompile(`https?://[a-zA-Z0-9.\-_~%#?&=/]+`)

	// Map to track wrapped URLs in this email
	urlMap := make(map[string]string)

	// Helper to process a text body
	processBody := func(body string) string {
		return reLink.ReplaceAllStringFunc(body, func(rawURL string) string {
			// Clean trailing punctuation that might be captured by regex in plain text
			cleanURL := rawURL
			var suffix string
			for len(cleanURL) > 0 {
				lastChar := cleanURL[len(cleanURL)-1]
				if lastChar == '.' || lastChar == ',' || lastChar == '?' || lastChar == '!' || lastChar == ')' || lastChar == ';' {
					suffix = string(lastChar) + suffix
					cleanURL = cleanURL[:len(cleanURL)-1]
				} else {
					break
				}
			}

			u, err := url.Parse(cleanURL)
			if err != nil {
				return rawURL
			}

			// Skip if it is already our short link host or is localhost/internal
			uHost := u.Host
			if idx := strings.IndexByte(uHost, ':'); idx >= 0 {
				uHost = uHost[:idx]
			}
			if strings.EqualFold(uHost, shortHost) || strings.EqualFold(uHost, "localhost") || strings.EqualFold(uHost, "127.0.0.1") {
				return rawURL
			}

			// Check if we already wrapped this URL in this email
			if cached, ok := urlMap[cleanURL]; ok {
				return cached + suffix
			}

			// Generate a unique slug
			var slug string
			for i := 0; i < 5; i++ {
				slug = randomSlug(6)
				if !p.isReservedSlug(slug) {
					var count int64
					p.db.Model(&models.Link{}).Where("slug = ?", slug).Count(&count)
					if count == 0 {
						break
					}
				}
			}

			// Create the link record
			link := models.Link{
				OrgID:   orgID,
				Host:    "", // host-agnostic
				Slug:    slug,
				Target:  cleanURL,
				Title:   "Auto-wrapped from outbound email",
				Enabled: true,
			}
			if err := p.db.Create(&link).Error; err != nil {
				return rawURL
			}

			shortURL := fmt.Sprintf("%s://%s/%s", scheme, shortHost, slug)
			urlMap[cleanURL] = shortURL
			return shortURL + suffix
		})
	}

	if msg.Text != "" {
		msg.Text = processBody(msg.Text)
	}
	if msg.HTML != "" {
		msg.HTML = processBody(msg.HTML)
	}
}

// POST /api/webhook/{orgSlug}/email/bounce/{token}

type bounceEvent struct {
	Email   string
	Event   string // "bounce" or "complaint"
	Details string
}

// isAWSSNSURL reports whether u is a legitimate AWS SNS confirmation URL: https
// to an sns.<region>.amazonaws.com host. This blocks the SubscribeURL (which is
// attacker-influenced) from pointing the server at arbitrary/internal hosts.
func isAWSSNSURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return strings.HasPrefix(host, "sns.") && strings.HasSuffix(host, ".amazonaws.com")
}

func extractBounceEvents(body []byte) []bounceEvent {
	var events []bounceEvent

	parseMap := func(m map[string]any) []bounceEvent {
		var results []bounceEvent

		// 1. SES Format
		if nType, ok := m["notificationType"].(string); ok && (nType == "Bounce" || nType == "Complaint") {
			if nType == "Bounce" {
				if bMap, ok := m["bounce"].(map[string]any); ok {
					bType, _ := bMap["bounceType"].(string)
					bSubType, _ := bMap["bounceSubType"].(string)
					details := fmt.Sprintf("Bounce Type: %s, SubType: %s", bType, bSubType)
					if recs, ok := bMap["bouncedRecipients"].([]any); ok {
						for _, rVal := range recs {
							if rMap, ok := rVal.(map[string]any); ok {
								if email, ok := rMap["emailAddress"].(string); ok {
									results = append(results, bounceEvent{
										Email:   email,
										Event:   "bounce",
										Details: details,
									})
								}
							}
						}
					}
				}
			} else if nType == "Complaint" {
				if cMap, ok := m["complaint"].(map[string]any); ok {
					feed, _ := cMap["complaintFeedbackType"].(string)
					details := fmt.Sprintf("Complaint Feedback Type: %s", feed)
					if recs, ok := cMap["complainedRecipients"].([]any); ok {
						for _, rVal := range recs {
							if rMap, ok := rVal.(map[string]any); ok {
								if email, ok := rMap["emailAddress"].(string); ok {
									results = append(results, bounceEvent{
										Email:   email,
										Event:   "complaint",
										Details: details,
									})
								}
							}
						}
					}
				}
			}
			if len(results) > 0 {
				return results
			}
		}

		// 2. Mailgun Format
		if edVal, ok := m["event-data"].(map[string]any); ok {
			ev, _ := edVal["event"].(string)
			recipient, _ := edVal["recipient"].(string)
			var details string
			if dsVal, ok := edVal["delivery-status"].(map[string]any); ok {
				if desc, ok := dsVal["description"].(string); ok {
					details = desc
				} else if msg, ok := dsVal["message"].(string); ok {
					details = msg
				}
			}
			if ev == "failed" || ev == "complained" {
				var finalEv string
				if ev == "failed" {
					finalEv = "bounce"
				} else {
					finalEv = "complaint"
				}
				if recipient != "" {
					results = append(results, bounceEvent{
						Email:   recipient,
						Event:   finalEv,
						Details: details,
					})
					return results
				}
			}
		}

		// 3. SendGrid / Generic Format
		var email, event, details string
		for _, key := range []string{"email", "recipient", "address", "rcpt"} {
			if eVal, ok := m[key].(string); ok && eVal != "" {
				email = eVal
				break
			}
		}
		for _, key := range []string{"event", "eventType"} {
			if eVal, ok := m[key].(string); ok && eVal != "" {
				event = eVal
				break
			}
		}
		for _, key := range []string{"reason", "description", "status"} {
			if eVal, ok := m[key].(string); ok && eVal != "" {
				details = eVal
				break
			}
		}
		event = strings.ToLower(event)
		if strings.Contains(event, "bounce") || event == "dropped" || event == "failed" {
			event = "bounce"
		} else if strings.Contains(event, "complaint") || event == "spamreport" {
			event = "complaint"
		}

		if email != "" && event != "" {
			results = append(results, bounceEvent{
				Email:   email,
				Event:   event,
				Details: details,
			})
		}
		return results
	}

	var list []map[string]any
	if err := json.Unmarshal(body, &list); err == nil {
		for _, m := range list {
			events = append(events, parseMap(m)...)
		}
		return events
	}

	var m map[string]any
	if err := json.Unmarshal(body, &m); err == nil {
		events = append(events, parseMap(m)...)
	}

	return events
}

func (p *Plugin) emailBelongsToOrg(emailID, orgID uint) bool {
	var e models.Email
	if p.db.Select("mailbox_id").First(&e, emailID).Error != nil {
		return false
	}
	var mb models.Mailbox
	if p.db.Select("owner_id").First(&mb, e.MailboxID).Error != nil {
		return false
	}
	return mb.OrgID == orgID
}

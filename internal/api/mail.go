package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
	"github.com/octarq-org/octarq/plugin"
)

// --- mailboxes ---

func (h *Handler) listMailboxes(w http.ResponseWriter, r *http.Request) {
	var boxes []models.Mailbox
	h.orgDB(r).Order("created_at DESC").Find(&boxes)
	// Attach unread counts.
	for i := range boxes {
		h.db.Model(&models.Email{}).
			Where("mailbox_id = ? AND read = ?", boxes[i].ID, false).
			Count(&boxes[i].Unread)
	}
	writeJSON(w, http.StatusOK, boxes)
}

type mailboxDTO struct {
	Address string `json:"address"`
	Note    string `json:"note"`
	Enabled *bool  `json:"enabled"`
}

func (h *Handler) createMailbox(w http.ResponseWriter, r *http.Request) {
	var d mailboxDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Address = strings.TrimSpace(strings.ToLower(d.Address))
	if !strings.Contains(d.Address, "@") {
		writeErr(w, http.StatusBadRequest, "address must be a full email")
		return
	}
	enabled := true
	if d.Enabled != nil {
		enabled = *d.Enabled
	}
	mb := models.Mailbox{
		OrgID:   h.orgID(r),
		Address: d.Address, Note: d.Note, Enabled: enabled,
	}
	if err := h.db.Create(&mb).Error; err != nil {
		writeErr(w, http.StatusConflict, "mailbox already exists")
		return
	}
	h.audit(r, "mailbox.create", "mailbox", mb.ID, map[string]any{"address": mb.Address})
	writeJSON(w, http.StatusCreated, mb)
}

func (h *Handler) updateMailbox(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var mb models.Mailbox
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&mb).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var d mailboxDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	mb.Note = d.Note
	if d.Enabled != nil {
		mb.Enabled = *d.Enabled
	}
	h.db.Save(&mb)
	meta := map[string]any{
		"note":    mb.Note,
		"enabled": mb.Enabled,
	}
	h.audit(r, "mailbox.update", "mailbox", mb.ID, meta)
	writeJSON(w, http.StatusOK, mb)
}

func (h *Handler) deleteMailbox(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	res := h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).Delete(&models.Mailbox{})
	if res.RowsAffected == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.db.Where("mailbox_id = ?", id).Delete(&models.Email{})
	h.audit(r, "mailbox.delete", "mailbox", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- emails ---

func (h *Handler) listEmails(w http.ResponseWriter, r *http.Request) {
	orgMailboxes := h.db.Model(&models.Mailbox{}).Select("id").Where("owner_id = ?", h.orgID(r))
	q := h.db.Where("mailbox_id IN (?)", orgMailboxes).Order("received_at DESC").Omit("Raw", "HTML")
	if mb := r.URL.Query().Get("mailbox"); mb != "" {
		q = q.Where("mailbox_id = ?", mb)
	}
	if s := r.URL.Query().Get("q"); s != "" {
		like := "%" + s + "%"
		q = q.Where("subject LIKE ? OR from_addr LIKE ? OR text LIKE ? OR note LIKE ?", like, like, like, like)
	}
	limit := 50
	if l, _ := strconv.Atoi(r.URL.Query().Get("limit")); l > 0 && l <= 500 {
		limit = l
	}
	offset := 0
	if o, _ := strconv.Atoi(r.URL.Query().Get("offset")); o > 0 {
		offset = o
	}
	q = q.Limit(limit).Offset(offset)
	var emails []models.Email
	q.Find(&emails)
	writeJSON(w, http.StatusOK, emails)
}

func (h *Handler) getEmail(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if !h.emailBelongsToOrg(id, h.orgID(r)) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var e models.Email
	if h.db.First(&e, id).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if !e.Read {
		h.db.Model(&e).Update("read", true)
		e.Read = true
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *Handler) updateEmail(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if !h.emailBelongsToOrg(id, h.orgID(r)) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var e models.Email
	if h.db.First(&e, id).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var d struct {
		Read *bool   `json:"read"`
		Note *string `json:"note"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if d.Read != nil {
		e.Read = *d.Read
	}
	if d.Note != nil {
		e.Note = *d.Note
	}
	h.db.Save(&e)
	writeJSON(w, http.StatusOK, e)
}

// readAllEmails marks every email read, optionally scoped to one mailbox.
func (h *Handler) readAllEmails(w http.ResponseWriter, r *http.Request) {
	orgMailboxes := h.db.Model(&models.Mailbox{}).Select("id").Where("owner_id = ?", h.orgID(r))
	q := h.db.Model(&models.Email{}).Where("read = ? AND mailbox_id IN (?)", false, orgMailboxes)
	if mb := r.URL.Query().Get("mailbox"); mb != "" {
		q = q.Where("mailbox_id = ?", mb)
	}
	res := q.Update("read", true)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": res.RowsAffected})
}

// rawEmail streams the original RFC822 message as a downloadable .eml file.
func (h *Handler) rawEmail(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if !h.emailBelongsToOrg(id, h.orgID(r)) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var e models.Email
	if h.db.First(&e, id).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", "message/rfc822")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"email-%d.eml\"", e.ID))
	w.Write(e.Raw)
}

func (h *Handler) deleteEmail(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if !h.emailBelongsToOrg(id, h.orgID(r)) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.db.Delete(&models.Email{}, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) sendEmail(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		mail.Message
		SMTPSenderID uint `json:"smtpSenderId"`
		TrackLinks   bool `json:"trackLinks"`
	}
	if err := readJSON(r, &payload); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(payload.To) == 0 {
		writeErr(w, http.StatusBadRequest, "to is required")
		return
	}

	// Rate-limit outbound mail per org so a leaked API token or a runaway client
	// can't burn the sending domain's / relay IP's reputation into an RBL.
	orgKey := fmt.Sprintf("org:%d", h.orgID(r))
	if !h.sendLimiter.allow(orgKey) {
		writeErr(w, http.StatusTooManyRequests, "send rate limit exceeded (max 100/hour) — try again later")
		return
	}

	var sender mail.Sender
	if payload.SMTPSenderID == 0 {
		writeErr(w, http.StatusBadRequest, "no SMTP sender selected")
		return
	}
	var s models.SMTPSender
	if err := h.db.Where("id = ? AND owner_id = ?", payload.SMTPSenderID, h.orgID(r)).First(&s).Error; err != nil {
		writeErr(w, http.StatusBadRequest, "invalid smtp sender id")
		return
	}
	pass, err := h.cipher.Decrypt(s.Pass)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "decrypt failed")
		return
	}
	// Force the From header to the sender's verified address — never trust a
	// client-supplied From, which would let a caller spoof arbitrary senders
	// through the relay.
	payload.Message.From = s.FromEmail
	sender = mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)

	if payload.TrackLinks {
		h.wrapLinksInEmail(r, &payload.Message)
	}

	if err := sender.Send(payload.Message); err != nil {
		writeErr(w, http.StatusBadRequest, "send failed: "+err.Error())
		return
	}
	h.sendLimiter.recordFailure(orgKey) // count this send against the per-org cap
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- inbound webhook (Cloudflare Email Routing -> Worker -> here) ---
//
// The Worker POSTs the raw RFC822 message body with header X-Octarq-Token.
// We parse it, match (or catch-all create) a mailbox by recipient, and store.

func (h *Handler) inbound(w http.ResponseWriter, r *http.Request) {
	// The {orgSlug} path segment names the tenant: a shared inbound host can't be
	// told apart by Host, so delivery is confined to this org's mailboxes.
	var org models.Org
	if h.db.Where("slug = ?", r.PathValue("orgSlug")).First(&org).Error != nil {
		writeErr(w, http.StatusNotFound, "unknown org")
		return
	}
	// Auth is the org's per-tenant token, carried in the path so the Cloudflare
	// worker needs only this one URL and no custom header.
	if org.InboundToken == "" || !secureEqual(r.PathValue("token"), org.InboundToken) {
		writeErr(w, http.StatusUnauthorized, "bad token")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 25<<20)) // 25 MiB cap
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body")
		return
	}
	parsed, _ := mail.Parse(raw)

	// The Worker may pass the intended recipient explicitly (more reliable than
	// the To header after routing).
	to := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Octarq-To")))
	if to == "" {
		to = strings.ToLower(parsed.To)
	}

	mb, ok := h.resolveMailbox(org.ID, to)
	if !ok {
		// Unknown recipient and catch-all disabled: accept silently so the
		// Worker doesn't bounce, but drop.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stored": false})
		return
	}

	att := ""
	if len(parsed.Attachments) > 0 {
		if b, err := json.Marshal(parsed.Attachments); err == nil {
			att = string(b)
		}
	}
	e := models.Email{
		MailboxID: mb.ID, MessageID: parsed.MessageID,
		FromAddr: parsed.From, ToAddr: to, Subject: parsed.Subject,
		Text: parsed.Text, HTML: parsed.HTML, Raw: parsed.Raw,
		Attachments: att, ReceivedAt: parsed.ReceivedAt,
		AuthSPF: parsed.Auth.SPF, AuthDKIM: parsed.Auth.DKIM, AuthDMARC: parsed.Auth.DMARC,
	}
	h.db.Create(&e)

	// Trigger Webhook Event Bus
	eventbus.Publish(mb.OrgID, "email.receive", map[string]any{
		"emailId":    e.ID,
		"mailboxId":  mb.ID,
		"from":       e.FromAddr,
		"to":         e.ToAddr,
		"subject":    e.Subject,
		"receivedAt": e.ReceivedAt,
	})

	// Fire the inbound-email hook so Pro plugins (Inbox AI) can summarize,
	// classify, or extract OTPs the moment mail lands. Dispatch is async per
	// handler, so this never blocks or fails the webhook.
	h.emitEmail(plugin.EmailEvent{
		ID:         e.ID,
		MailboxID:  mb.ID,
		OrgID:      mb.OrgID,
		From:       parsed.From,
		To:         to,
		Subject:    parsed.Subject,
		Text:       parsed.Text,
		HTML:       parsed.HTML,
		ReceivedAt: parsed.ReceivedAt,
	})

	// Best-effort notification; never block or fail the webhook.
	text := fmt.Sprintf("📧 New mail to %s — From: %s — %s", to, parsed.From, parsed.Subject)
	var channels []models.NotificationChannel
	h.db.Where("owner_id = ? AND enabled = ?", mb.OrgID, true).Find(&channels)
	if len(channels) > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, ch := range channels {
				_ = notify.Send(ctx, ch.Type, ch.Config, text)
			}
		}()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stored": true, "id": e.ID})
}

// mailHostDisabled reports whether host is listed as a mail host on some domain
// but every such listing is disabled (so mail to it should be dropped).
func (h *Handler) mailHostDisabled(host string) bool {
	var doms []models.Domain
	h.db.Where("for_mail = ?", true).Find(&doms)
	listed := false
	for _, d := range doms {
		for _, mh := range d.MailHosts {
			if mh.Host == host {
				listed = true
				if mh.Enabled {
					return false
				}
			}
		}
	}
	return listed
}

// resolveMailbox finds an enabled mailbox for the address within the given org,
// optionally creating one when catch-all is on and the recipient's domain (also
// owned by that org) is managed for mail. Scoping by org keeps one tenant's
// inbound webhook from delivering into another tenant's mailboxes.
func (h *Handler) resolveMailbox(orgID uint, addr string) (*models.Mailbox, bool) {
	if addr == "" {
		return nil, false
	}
	// Drop mail to a temporarily disabled mail host, even for existing mailboxes.
	if at := strings.LastIndex(addr, "@"); at >= 0 && h.mailHostDisabled(addr[at+1:]) {
		return nil, false
	}
	var mb models.Mailbox
	if err := h.db.Where("address = ? AND enabled = ? AND owner_id = ?", addr, true, orgID).First(&mb).Error; err == nil {
		return &mb, true
	}
	if h.GetWorkspaceSetting(orgID, keyCatchAll) != "true" {
		return nil, false
	}
	// Reserved local-parts are never auto-created by catch-all.
	if h.isReservedMailbox(orgID, addr) {
		return nil, false
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return nil, false
	}
	recipientHost := addr[at+1:]
	// The recipient host must be one of THIS org's mail-enabled domain's mail
	// hosts (apex or a configured subdomain like mail.example.com).
	var doms []models.Domain
	h.db.Where("for_mail = ? AND owner_id = ?", true, orgID).Find(&doms)
	var matched bool
	for _, dom := range doms {
		for _, mh := range dom.EffectiveMailHosts() {
			if mh == recipientHost {
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	if !matched {
		return nil, false
	}
	mb = models.Mailbox{OrgID: orgID, Address: addr, Enabled: true, Note: "auto (catch-all)"}
	if err := h.db.Create(&mb).Error; err != nil {
		return nil, false
	}
	return &mb, true
}

func (h *Handler) wrapLinksInEmail(r *http.Request, msg *mail.Message) {
	orgID := h.orgID(r)

	// Determine the short link domain host
	var doms []models.Domain
	h.db.Where("owner_id = ? AND for_link = ?", orgID, true).Find(&doms)
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
				if !h.isReservedSlug(slug) {
					var count int64
					h.db.Model(&models.Link{}).Where("slug = ?", slug).Count(&count)
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
			if err := h.db.Create(&link).Error; err != nil {
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
func (h *Handler) emailBounceWebhook(w http.ResponseWriter, r *http.Request) {
	// Authenticate + scope by tenant (same scheme as inbound): the {orgSlug}
	// names the org, the {token} must match its per-org secret. Without this a
	// forged POST could spam an org's notification channels and audit log.
	var org models.Org
	if h.db.Where("slug = ?", r.PathValue("orgSlug")).First(&org).Error != nil {
		writeErr(w, http.StatusNotFound, "unknown org")
		return
	}
	if org.InboundToken == "" || !secureEqual(r.PathValue("token"), org.InboundToken) {
		writeErr(w, http.StatusUnauthorized, "bad token")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20)) // 5 MiB cap
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body")
		return
	}

	// Check for AWS SNS wrapped payload
	var snsWrap map[string]any
	if err := json.Unmarshal(body, &snsWrap); err == nil {
		if snsType, ok := snsWrap["Type"].(string); ok {
			if snsType == "SubscriptionConfirmation" {
				if subURL, ok := snsWrap["SubscribeURL"].(string); ok && subURL != "" {
					// SSRF guard: only auto-confirm to a genuine AWS SNS endpoint
					// over https, and fetch through the SSRF-safe client (blocks
					// private/loopback/metadata IPs). SubscribeURL is attacker-
					// influenced, so it must never be fetched blindly.
					if !isAWSSNSURL(subURL) {
						log.Printf("bounce: refusing SNS SubscribeURL with non-AWS host: %s", subURL)
						writeErr(w, http.StatusBadRequest, "invalid SubscribeURL")
						return
					}
					go func() {
						resp, err := safeGet(context.Background(), subURL)
						if err == nil {
							resp.Body.Close()
							log.Printf("AWS SNS subscription confirmed successfully")
						} else {
							log.Printf("AWS SNS subscription confirmation failed: %v", err)
						}
					}()
					writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Subscription confirmation triggered"})
					return
				}
			}
			if snsType == "Notification" {
				if msgStr, ok := snsWrap["Message"].(string); ok && msgStr != "" {
					// Replace body with the actual inner message bytes
					body = []byte(msgStr)
				}
			}
		}
	}

	events := extractBounceEvents(body)
	if len(events) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "processed": 0})
		return
	}

	ip := reporterIP(r)
	processedCount := 0

	for _, ev := range events {
		var mb models.Mailbox
		if err := h.db.Where("address = ? AND owner_id = ?", strings.ToLower(ev.Email), org.ID).First(&mb).Error; err != nil {
			continue
		}

		processedCount++

		// Write Audit Log
		meta := map[string]any{
			"address": ev.Email,
			"event":   ev.Event,
			"details": ev.Details,
		}
		var metaJSON string
		if b, err := json.Marshal(meta); err == nil {
			metaJSON = string(b)
		}

		h.db.Create(&models.AuditLog{
			OrgID:      mb.OrgID,
			ActorID:    0, // System
			Action:     "email.bounce",
			TargetType: "mailbox",
			TargetID:   mb.ID,
			Meta:       metaJSON,
			IP:         ip,
		})

		// Send alert (notifications)
		alertText := fmt.Sprintf("⚠️ Email reputation event: Mailbox %s experienced a %s event. Details: %s", mb.Address, ev.Event, ev.Details)
		var channels []models.NotificationChannel
		h.db.Where("owner_id = ? AND enabled = ?", mb.OrgID, true).Find(&channels)
		if len(channels) > 0 {
			go func(chans []models.NotificationChannel, txt string) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				for _, ch := range chans {
					_ = notify.Send(ctx, ch.Type, ch.Config, txt)
				}
			}(channels, alertText)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "processed": processedCount})
}

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

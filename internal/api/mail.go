package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Jungley8/led/internal/eventbus"
	"github.com/Jungley8/led/internal/mail"
	"github.com/Jungley8/led/internal/models"
	"github.com/Jungley8/led/internal/notify"
	"github.com/Jungley8/led/plugin"
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
		OrgID: h.orgID(r),
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
	h.audit(r, "mailbox.update", "mailbox", mb.ID, nil)
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
	sender = mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)

	if payload.TrackLinks {
		h.wrapLinksInEmail(r, &payload.Message)
	}

	if err := sender.Send(payload.Message); err != nil {
		writeErr(w, http.StatusBadRequest, "send failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- inbound webhook (Cloudflare Email Routing -> Worker -> here) ---
//
// The Worker POSTs the raw RFC822 message body with header X-Led-Token.
// We parse it, match (or catch-all create) a mailbox by recipient, and store.

func (h *Handler) inbound(w http.ResponseWriter, r *http.Request) {
	inboundToken := h.getSetting(keyInboundToken)
	if inboundToken == "" || r.Header.Get("X-Led-Token") != inboundToken {
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
	to := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Led-To")))
	if to == "" {
		to = strings.ToLower(parsed.To)
	}

	mb, ok := h.resolveMailbox(to)
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
	h.db.Where("enabled = ?", true).Find(&channels)
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

// resolveMailbox finds an enabled mailbox for the address, optionally creating
// one when catch-all is on and the recipient's domain is managed for mail.
func (h *Handler) resolveMailbox(addr string) (*models.Mailbox, bool) {
	if addr == "" {
		return nil, false
	}
	// Drop mail to a temporarily disabled mail host, even for existing mailboxes.
	if at := strings.LastIndex(addr, "@"); at >= 0 && h.mailHostDisabled(addr[at+1:]) {
		return nil, false
	}
	var mb models.Mailbox
	if err := h.db.Where("address = ? AND enabled = ?", addr, true).First(&mb).Error; err == nil {
		return &mb, true
	}
	if h.getSetting(keyCatchAll) != "true" {
		return nil, false
	}
	// Reserved local-parts are never auto-created by catch-all.
	if h.isReservedMailbox(addr) {
		return nil, false
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return nil, false
	}
	recipientHost := addr[at+1:]
	// The recipient host must be one of a mail-enabled domain's mail hosts
	// (apex or a configured subdomain like mail.example.com).
	var doms []models.Domain
	h.db.Where("for_mail = ?", true).Find(&doms)
	var matchedOrgID uint
	for _, dom := range doms {
		for _, mh := range dom.EffectiveMailHosts() {
			if mh == recipientHost {
				matchedOrgID = dom.OrgID
				break
			}
		}
		if matchedOrgID != 0 {
			break
		}
	}
	if matchedOrgID == 0 {
		return nil, false
	}
	mb = models.Mailbox{OrgID: matchedOrgID, Address: addr, Enabled: true, Note: "auto (catch-all)"}
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

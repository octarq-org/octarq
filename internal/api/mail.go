package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jungley/led/internal/mail"
	"github.com/jungley/led/internal/models"
)

// --- mailboxes ---

func (h *Handler) listMailboxes(w http.ResponseWriter, r *http.Request) {
	var boxes []models.Mailbox
	h.db.Order("created_at DESC").Find(&boxes)
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
		OwnerID: models.SingleUserID,
		Address: d.Address, Note: d.Note, Enabled: enabled,
	}
	if err := h.db.Create(&mb).Error; err != nil {
		writeErr(w, http.StatusConflict, "mailbox already exists")
		return
	}
	writeJSON(w, http.StatusCreated, mb)
}

func (h *Handler) updateMailbox(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var mb models.Mailbox
	if h.db.First(&mb, id).Error != nil {
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
	writeJSON(w, http.StatusOK, mb)
}

func (h *Handler) deleteMailbox(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	h.db.Where("mailbox_id = ?", id).Delete(&models.Email{})
	h.db.Delete(&models.Mailbox{}, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- emails ---

func (h *Handler) listEmails(w http.ResponseWriter, r *http.Request) {
	q := h.db.Order("received_at DESC").Omit("Raw", "HTML")
	if mb := r.URL.Query().Get("mailbox"); mb != "" {
		q = q.Where("mailbox_id = ?", mb)
	}
	if s := r.URL.Query().Get("q"); s != "" {
		like := "%" + s + "%"
		q = q.Where("subject LIKE ? OR from_addr LIKE ? OR text LIKE ? OR note LIKE ?", like, like, like, like)
	}
	var emails []models.Email
	q.Limit(200).Find(&emails)
	writeJSON(w, http.StatusOK, emails)
}

func (h *Handler) getEmail(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
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
	q := h.db.Model(&models.Email{}).Where("read = ?", false)
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
	h.db.Delete(&models.Email{}, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) sendEmail(w http.ResponseWriter, r *http.Request) {
	if h.sender == nil {
		writeErr(w, http.StatusServiceUnavailable, "no SMTP relay configured (set LED_SMTP_*)")
		return
	}
	var m mail.Message
	if err := readJSON(r, &m); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(m.To) == 0 {
		writeErr(w, http.StatusBadRequest, "to is required")
		return
	}
	if err := h.sender.Send(m); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- inbound webhook (Cloudflare Email Routing -> Worker -> here) ---
//
// The Worker POSTs the raw RFC822 message body with header X-Led-Token.
// We parse it, match (or catch-all create) a mailbox by recipient, and store.

func (h *Handler) inbound(w http.ResponseWriter, r *http.Request) {
	if h.cfg.InboundToken == "" || r.Header.Get("X-Led-Token") != h.cfg.InboundToken {
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
	}
	h.db.Create(&e)

	// Best-effort Telegram notification; never block or fail the webhook.
	if h.notifier != nil {
		text := fmt.Sprintf("📧 New mail to %s — From: %s — %s", to, parsed.From, parsed.Subject)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = h.notifier.Notify(ctx, text)
		}()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stored": true, "id": e.ID})
}

// resolveMailbox finds an enabled mailbox for the address, optionally creating
// one when catch-all is on and the recipient's domain is managed for mail.
func (h *Handler) resolveMailbox(addr string) (*models.Mailbox, bool) {
	if addr == "" {
		return nil, false
	}
	var mb models.Mailbox
	if err := h.db.Where("address = ? AND enabled = ?", addr, true).First(&mb).Error; err == nil {
		return &mb, true
	}
	if !h.cfg.CatchAll {
		return nil, false
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return nil, false
	}
	domain := addr[at+1:]
	var dom models.Domain
	if h.db.Where("name = ? AND for_mail = ?", domain, true).First(&dom).Error != nil {
		return nil, false
	}
	mb = models.Mailbox{OwnerID: models.SingleUserID, Address: addr, Enabled: true, Note: "auto (catch-all)"}
	if err := h.db.Create(&mb).Error; err != nil {
		return nil, false
	}
	return &mb, true
}

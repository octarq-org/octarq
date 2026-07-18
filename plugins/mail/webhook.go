package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"crypto/subtle"

	"github.com/octarq-org/octarq/internal/safehttp"

	"time"

	"github.com/octarq-org/octarq/plugins/dns"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

// --- mailboxes ---

// --- emails ---

// readAllEmails marks every email read, optionally scoped to one mailbox.

// rawEmail streams the original RFC822 message as a downloadable .eml file.

// --- inbound webhook (Cloudflare Email Routing -> Worker -> here) ---
//
// The Worker POSTs the raw RFC822 message body with header X-Octarq-Token.
// We parse it, match (or catch-all create) a mailbox by recipient, and store.

type InboundInput struct {
	Ctx     huma.Context `hidden:"true"`
	OrgSlug string       `path:"orgSlug"`
	Token   string       `path:"token"`
}

func (i *InboundInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type InboundOutput struct {
	Body map[string]any
}

func (p *Plugin) inbound(ctx context.Context, input *InboundInput) (*InboundOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	// The {orgSlug} path segment names the tenant: a shared inbound host can't be
	// told apart by Host, so delivery is confined to this org's mailboxes.
	var org models.Org
	if p.db.Where("slug = ?", input.OrgSlug).First(&org).Error != nil {
		return nil, huma.Error404NotFound("unknown org")
	}
	// Auth is the org's per-tenant token, carried in the path so the Cloudflare
	// worker needs only this one URL and no custom header.
	if org.InboundToken == "" || subtle.ConstantTimeCompare([]byte(input.Token), []byte(org.InboundToken)) != 1 {
		return nil, huma.Error401Unauthorized("bad token")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 25<<20)) // 25 MiB cap
	if err != nil {
		return nil, huma.Error400BadRequest("read body")
	}
	parsed, _ := mail.Parse(raw)

	// The Worker may pass the intended recipient explicitly (more reliable than
	// the To header after routing).
	to := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Octarq-To")))
	if to == "" {
		to = strings.ToLower(parsed.To)
	}

	mb, ok := p.resolveMailbox(org.ID, to)
	if !ok {
		// Unknown recipient and catch-all disabled: accept silently so the
		// Worker doesn't bounce, but drop.
		return &InboundOutput{Body: map[string]any{"ok": true, "stored": false}}, nil
	}

	att := ""
	if len(parsed.Attachments) > 0 {
		if b, err := json.Marshal(parsed.Attachments); err == nil {
			att = string(b)
		}
	}
	e := Email{
		MailboxID: mb.ID, MessageID: parsed.MessageID,
		FromAddr: parsed.From, ToAddr: to, Subject: parsed.Subject,
		Text: parsed.Text, HTML: parsed.HTML, Raw: parsed.Raw,
		Attachments: att, ReceivedAt: parsed.ReceivedAt,
		AuthSPF: parsed.Auth.SPF, AuthDKIM: parsed.Auth.DKIM, AuthDMARC: parsed.Auth.DMARC,
	}
	p.db.Create(&e)

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
	p.emitEmail(plugin.EmailEvent{
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
	p.db.Where("owner_id = ? AND enabled = ?", mb.OrgID, true).Find(&channels)
	if len(channels) > 0 {
		go func() {
			ctxCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, ch := range channels {
				var cfg map[string]any
				json.Unmarshal([]byte(ch.Config), &cfg)
				_ = p.notify(ctxCtx, ch.Type, cfg, text)
			}
		}()
	}

	return &InboundOutput{Body: map[string]any{"ok": true, "stored": true, "id": e.ID}}, nil
}

// mailHostDisabled reports whether host is listed as a mail host on some domain
// but every such listing is disabled (so mail to it should be dropped).
func (p *Plugin) mailHostDisabled(host string) bool {
	var doms []dns.Domain
	p.db.Find(&doms)
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
func (p *Plugin) resolveMailbox(orgID uint, addr string) (*Mailbox, bool) {
	if addr == "" {
		return nil, false
	}
	// Drop mail to a temporarily disabled mail host, even for existing mailboxes.
	if at := strings.LastIndex(addr, "@"); at >= 0 && p.mailHostDisabled(addr[at+1:]) {
		return nil, false
	}
	var mb Mailbox
	if err := p.db.Where("address = ? AND enabled = ? AND owner_id = ?", addr, true, orgID).First(&mb).Error; err == nil {
		return &mb, true
	}
	if p.getWorkspaceSetting(orgID, "mail.catch_all") != "true" {
		return nil, false
	}
	// Reserved local-parts are never auto-created by catch-all.
	if p.isReservedMailbox(orgID, addr) {
		return nil, false
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return nil, false
	}
	recipientHost := addr[at+1:]
	// The recipient host must be one of THIS org's mail-enabled domain's mail
	// hosts (apex or a configured subdomain like mail.example.com).
	var doms []dns.Domain
	p.db.Where("owner_id = ?", orgID).Find(&doms)
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
	mb = Mailbox{OrgID: orgID, Address: addr, Enabled: true, Note: "auto (catch-all)"}
	if err := p.db.Create(&mb).Error; err != nil {
		return nil, false
	}
	return &mb, true
}

type EmailBounceWebhookInput struct {
	Ctx     huma.Context `hidden:"true"`
	OrgSlug string       `path:"orgSlug"`
	Token   string       `path:"token"`
}

func (i *EmailBounceWebhookInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type EmailBounceWebhookOutput struct {
	Body map[string]any
}

// POST /api/webhook/{orgSlug}/email/bounce/{token}
func (p *Plugin) emailBounceWebhook(ctx context.Context, input *EmailBounceWebhookInput) (*EmailBounceWebhookOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	// Authenticate + scope by tenant (same scheme as inbound): the {orgSlug}
	// names the org, the {token} must match its per-org secret. Without this a
	// forged POST could spam an org's notification channels and audit log.
	var org models.Org
	if p.db.Where("slug = ?", input.OrgSlug).First(&org).Error != nil {
		return nil, huma.Error404NotFound("unknown org")
	}
	if org.InboundToken == "" || subtle.ConstantTimeCompare([]byte(input.Token), []byte(org.InboundToken)) != 1 {
		return nil, huma.Error401Unauthorized("bad token")
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20)) // 5 MiB cap
	if err != nil {
		return nil, huma.Error400BadRequest("read body")
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
						return nil, huma.Error400BadRequest("invalid SubscribeURL")
					}
					go func() {
						resp, err := safehttp.Get(context.Background(), http.DefaultClient, subURL, "")
						if err == nil {
							resp.Body.Close()
							log.Printf("AWS SNS subscription confirmed successfully")
						} else {
							log.Printf("AWS SNS subscription confirmation failed: %v", err)
						}
					}()
					return &EmailBounceWebhookOutput{
						Body: map[string]any{"ok": true, "message": "Subscription confirmation triggered"},
					}, nil
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
		return &EmailBounceWebhookOutput{
			Body: map[string]any{"ok": true, "processed": 0},
		}, nil
	}

	ip := reporterIP(r)
	processedCount := 0

	for _, ev := range events {
		var mb Mailbox
		if err := p.db.Where("address = ? AND owner_id = ?", strings.ToLower(ev.Email), org.ID).First(&mb).Error; err != nil {
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

		p.db.Create(&models.AuditLog{
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
		p.db.Where("owner_id = ? AND enabled = ?", mb.OrgID, true).Find(&channels)
		if len(channels) > 0 {
			go func(chans []models.NotificationChannel, txt string) {
				ctxCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				for _, ch := range chans {
					var cfg map[string]any
					json.Unmarshal([]byte(ch.Config), &cfg)
					_ = p.notify(ctxCtx, ch.Type, cfg, txt)
				}
			}(channels, alertText)
		}
	}

	return &EmailBounceWebhookOutput{
		Body: map[string]any{"ok": true, "processed": processedCount},
	}, nil
}

// isAWSSNSURL reports whether u is a legitimate AWS SNS confirmation URL: https
// to an sns.<region>.amazonaws.com host. This blocks the SubscribeURL (which is
// attacker-influenced) from pointing the server at arbitrary/internal hosts.

func reporterIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if idx := strings.IndexByte(ip, ','); idx >= 0 {
			return strings.TrimSpace(ip[:idx])
		}
		return strings.TrimSpace(ip)
	}
	return r.RemoteAddr
}

func (p *Plugin) isReservedMailbox(orgID uint, addr string) bool {
	parts := strings.SplitN(addr, "@", 2)
	if len(parts) != 2 {
		return false
	}
	user := strings.ToLower(parts[0])
	reserved := []string{"admin", "administrator", "hostmaster", "postmaster", "webmaster"}
	for _, r := range reserved {
		if user == r {
			return true
		}
	}
	return false
}

func (p *Plugin) emitEmail(e plugin.EmailEvent) {
	p.emailMu.RLock()
	handlers := p.emailHandlers
	p.emailMu.RUnlock()
	for _, h := range handlers {
		if h != nil {
			go h(e)
		}
	}
}

func (p *Plugin) OnEmail(handler func(plugin.EmailEvent)) {
	if handler == nil {
		return
	}
	p.emailMu.Lock()
	defer p.emailMu.Unlock()
	p.emailHandlers = append(p.emailHandlers, handler)
}

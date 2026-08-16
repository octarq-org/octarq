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

	"github.com/octarq-org/octarq/internal/safego"
	"github.com/octarq-org/octarq/internal/safehttp"

	"time"

	"github.com/octarq-org/octarq/plugins/dns"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/usagemetric"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
		p.recordInboundAuthFailure(r, org.ID, "path-token")
		return nil, huma.Error401Unauthorized("bad token")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 25<<20)) // 25 MiB cap
	if err != nil {
		return nil, huma.Error400BadRequest("read body")
	}
	overrideTo := r.Header.Get("X-Octarq-To")
	return p.processInboundMail(ctx, org.ID, overrideTo, raw)
}

type InboundGenericInput struct {
	Ctx     huma.Context `hidden:"true"`
	OrgSlug string       `path:"orgSlug"`
	Token   string       `path:"token"`
}

func (i *InboundGenericInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (p *Plugin) inboundGeneric(ctx context.Context, input *InboundGenericInput) (*InboundOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)

	// Auth & Tenant check: look up org by slug, but return generic 401 for unknown org
	// so no org existence is leaked in HTTP responses.
	var org models.Org
	if p.db.Where("slug = ?", input.OrgSlug).First(&org).Error != nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Auth is the org token, in the path. This endpoint exists for providers
	// where the only thing you can configure is a URL — SendGrid Inbound Parse,
	// Mailgun routes — so demanding a custom header would defeat its purpose.
	// It is the same shape n8n and every other webhook receiver uses, and it
	// rotates from Mail settings (an empty inboundToken there mints a fresh
	// UUID), which is what makes a URL-borne secret workable: if it leaks, you
	// change it and repaste one URL.
	if org.InboundToken == "" || subtle.ConstantTimeCompare([]byte(input.Token), []byte(org.InboundToken)) != 1 {
		p.recordInboundAuthFailure(r, org.ID, "generic")
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	raw, err := extractRawEmail(r)
	if err != nil || len(raw) == 0 {
		return nil, huma.Error400BadRequest("read body")
	}

	overrideTo := r.Header.Get("X-Octarq-To")
	if overrideTo == "" {
		overrideTo = r.Header.Get("X-Inbound-To")
	}

	return p.processInboundMail(ctx, org.ID, overrideTo, raw)
}

// recordInboundAuthFailure leaves a trace when someone presents the wrong
// inbound token.
//
// Both inbound routes and the bounce webhook are public by necessity — an MTA
// arrives with no session — and all three authenticate on the same org-wide
// InboundToken. Until this existed, a wrong token produced a bare 401: no audit
// row, no log line, no counter. Someone could grind the token space for a week
// and the only evidence would be the rate limiter's own bookkeeping, which
// nobody reads. Guessing that token buys the ability to inject mail into any
// mailbox in the workspace, so it is worth knowing that guessing is happening.
//
// The token itself is never recorded, in any form. An audit trail that stores
// attempted credentials becomes a credential store the moment someone
// mistypes their real token into the wrong tenant's URL.
func (p *Plugin) recordInboundAuthFailure(r *http.Request, orgID uint, route string) {
	ip := reporterIP(r)
	log.Printf("inbound: rejected bad token for org %d via %s from %s", orgID, route, ip)
	if p.audit == nil || r == nil {
		return
	}
	p.audit(r, "email.inbound.auth_failed", "org", orgID, map[string]any{
		"route": route,
		"ip":    ip,
	})
}

func extractRawEmail(r *http.Request) ([]byte, error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(ct), "multipart/form-data") {
		if err := r.ParseMultipartForm(25 << 20); err == nil && r.MultipartForm != nil {
			fieldNames := []string{"email", "raw", "body-mime", "message", "eml", "body"}
			for _, name := range fieldNames {
				if files, ok := r.MultipartForm.File[name]; ok && len(files) > 0 {
					f, err := files[0].Open()
					if err == nil {
						b, err := io.ReadAll(io.LimitReader(f, 25<<20))
						_ = f.Close()
						if err == nil && len(b) > 0 {
							return b, nil
						}
					}
				}
			}
			for _, name := range fieldNames {
				if vals, ok := r.MultipartForm.Value[name]; ok && len(vals) > 0 && vals[0] != "" {
					return []byte(vals[0]), nil
				}
			}
		}
	}
	return io.ReadAll(io.LimitReader(r.Body, 25<<20))
}

func (p *Plugin) processInboundMail(ctx context.Context, orgID uint, overrideTo string, raw []byte) (*InboundOutput, error) {
	ctx = plugin.WithOrgID(ctx, orgID)
	parsed, parseErr := mail.Parse(raw)
	if parseErr != nil {
		log.Printf("inbound: mail parse failed: %v", parseErr)
	}

	to := strings.ToLower(strings.TrimSpace(overrideTo))
	if to == "" && parsed != nil {
		to = strings.ToLower(parsed.To)
	}

	mb, ok := p.resolveMailbox(orgID, to)
	if !ok {
		// Unknown recipient and catch-all disabled: accept silently so the
		// Worker/MTA doesn't bounce, but drop.
		return &InboundOutput{Body: map[string]any{"ok": true, "stored": false}}, nil
	}

	att := ""
	if parsed != nil && len(parsed.Attachments) > 0 {
		if b, err := json.Marshal(parsed.Attachments); err == nil {
			att = string(b)
		}
	}

	msgID := ""
	from := ""
	subject := ""
	textBody := ""
	htmlBody := ""
	var receivedAt time.Time
	spf, dkim, dmarc := "", "", ""

	if parsed != nil {
		msgID = parsed.MessageID
		from = parsed.From
		subject = parsed.Subject
		textBody = parsed.Text
		htmlBody = parsed.HTML
		receivedAt = parsed.ReceivedAt
		spf = parsed.Auth.SPF
		dkim = parsed.Auth.DKIM
		dmarc = parsed.Auth.DMARC
	}

	e := Email{
		MailboxID: mb.ID, MessageID: msgID,
		FromAddr: from, ToAddr: to, Subject: subject,
		Text: textBody, HTML: htmlBody,
		Attachments: att, ReceivedAt: receivedAt,
		AuthSPF: spf, AuthDKIM: dkim, AuthDMARC: dmarc,
	}
	if err := p.db.Create(&e).Error; err != nil {
		log.Printf("inbound: failed to store email: %v", err)
		return nil, huma.Error500InternalServerError("failed to store email")
	}

	// The RFC822 original goes through the storage seam rather than onto the
	// Email row: on the default database backend it lands in mail_raw_blobs,
	// which keeps the emails table (the one every list query scans) free of
	// multi-megabyte blobs. Email.Raw is only ever read now, never written —
	// it holds the originals of messages received before this seam existed.
	key := fmt.Sprintf("mail/%d/%d.eml", mb.OrgID, e.ID)
	storageProv, spErr := p.getStorageProvider()
	if spErr != nil {
		log.Printf("inbound: storage provider configuration error (%v); falling back to the database", spErr)
		storageProv = NewDBStorageProvider(p.db)
	}

	// A misconfigured or unreachable backend must never cost us the message —
	// it is the only copy. Fall back to the Email row and let the operator
	// notice through the log rather than dropping mail on the floor.
	if putErr := storageProv.Put(ctx, key, raw); putErr != nil {
		log.Printf("mail storage: Put failed for key %s, falling back to the database: %v", key, putErr)
		p.db.Model(&e).Update("raw", raw)
	} else {
		p.db.Model(&e).Update("storage_key", key)
	}

	// Bytes written, not bytes held: deletions and retention pruning do not come
	// back through here, so this counts inbound volume and will drift above the
	// true footprint. Billing storage by what an org currently occupies needs a
	// periodic Stat sweep over its keys — deliberately not built here.
	// mail.raw_bytes has no consumer on the quota side today (it is absent from
	// pkg/quota's metricNames); it is kept for future storage billing and must
	// not be deleted.
	if p.recordUsage != nil {
		p.recordUsage(mb.OrgID, usagemetric.RawBytes, int64(len(raw)))
		// Count the message itself as one unit of inbound mail. This is the
		// meter behind the mailInPerMonth quota key.
		p.recordUsage(mb.OrgID, usagemetric.MailIn, 1)
	}

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
		From:       from,
		To:         to,
		Subject:    subject,
		Text:       textBody,
		HTML:       htmlBody,
		ReceivedAt: receivedAt,
	})

	// Best-effort notification; never block or fail the webhook.
	text := fmt.Sprintf("📧 New mail to %s — From: %s — %s", to, from, subject)
	var channels []models.NotificationChannel
	p.db.Where("owner_id = ? AND enabled = ?", mb.OrgID, true).Find(&channels)
	if len(channels) > 0 {
		safego.Go("mail.inbound-notify", func() {
			ctxCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, ch := range channels {
				if p.notify != nil {
					_ = p.notify(ctxCtx, ch.Type, ch.Config, text)
				}
			}
		})
	}

	return &InboundOutput{Body: map[string]any{"ok": true, "stored": true, "id": e.ID}}, nil
}

// mailHostDisabled reports whether host is listed as a mail host on the owner's domain
// but every such listing is disabled (so mail to it should be dropped).
func (p *Plugin) mailHostDisabled(orgID uint, host string) bool {
	if orgID == 0 {
		return false
	}
	normHost := dns.NormalizeHost(host)
	var doms []dns.Domain
	p.db.Where("owner_id = ? AND for_mail = ?", orgID, true).Find(&doms)
	listed := false
	for _, d := range doms {
		for _, mh := range d.MailHosts {
			if dns.NormalizeHost(mh.Host) == normHost {
				listed = true
				if mh.Enabled {
					return false
				}
			}
		}
	}
	return listed
}

// mailAddressDomainNotAnotherTenants reports whether orgID may create a mailbox
// at addr — true unless addr's domain is a mail host belonging to a *different*
// workspace.
//
// Scope note, because the weaker half of this rule is deliberate. The defect
// being fixed is cross-tenant squatting: mailbox addresses were globally unique,
// so one workspace could take `billing@victim.com` and permanently block the
// tenant that actually owns victim.com from ever creating it. That is what this
// refuses.
//
// It does NOT require the domain to be one the workspace has already registered.
// Requiring that would be a stricter and arguably better rule — you cannot
// receive mail at a domain you have not set up — but it is a product behaviour
// change, not a security fix: creating a mailbox ahead of registering its domain
// works today and is exercised by existing tests. Delivery is unaffected either
// way; the inbound path is already gated on the org's own token and its own mail
// hosts (see resolveMailbox), so an unregistered mailbox simply never receives.
func (p *Plugin) mailAddressDomainNotAnotherTenants(orgID uint, addr string) bool {
	if orgID == 0 || p.db == nil {
		return false
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return false
	}
	normHost := dns.NormalizeHost(addr[at+1:])
	if normHost == "" {
		return false
	}
	var doms []dns.Domain
	if err := p.db.Where("for_mail = ?", true).Find(&doms).Error; err != nil {
		return false
	}
	for _, d := range doms {
		if d.OrgID == orgID {
			continue // your own domain never blocks you
		}
		for _, mh := range d.MailHosts {
			if dns.NormalizeHost(mh.Host) == normHost {
				return false // this hostname belongs to another workspace
			}
		}
	}
	return true
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
	if at := strings.LastIndex(addr, "@"); at >= 0 && p.mailHostDisabled(orgID, addr[at+1:]) {
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
	// The recipient host must be one of THIS org's mail-enabled domain's mail
	// hosts (apex or a configured subdomain like mail.example.com).
	//
	// Strict on purpose, and deliberately NOT the looser rule manual creation
	// uses: this path creates a mailbox by itself, from an inbound message, with
	// no operator in the loop. "Not another tenant's" is not enough when nobody
	// is choosing — anything routed at this org's webhook would materialise a
	// mailbox.
	if !p.ownsMailHost(orgID, addr) {
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
		p.recordInboundAuthFailure(r, org.ID, "bounce")
		return nil, huma.Error401Unauthorized("bad token")
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20)) // 5 MiB cap
	if err != nil {
		return nil, huma.Error400BadRequest("read body")
	}

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
					safego.Go("mail.sns-confirm", func() {
						resp, err := safehttp.Get(context.Background(), http.DefaultClient, subURL, "")
						if err == nil {
							resp.Body.Close()
							log.Printf("AWS SNS subscription confirmed successfully")
						} else {
							log.Printf("AWS SNS subscription confirmation failed: %v", err)
						}
					})
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

		shouldSuppress := false
		reason := ""
		if ev.Event == "complaint" {
			shouldSuppress = true
			reason = "complaint"
		} else if ev.Event == "bounce" && strings.EqualFold(ev.BounceType, "Permanent") {
			shouldSuppress = true
			reason = "hard_bounce"
		}

		if shouldSuppress {
			addr := strings.ToLower(strings.TrimSpace(ev.Email))
			if addr != "" {
				item := MailSuppression{
					OrgID:     org.ID,
					Address:   addr,
					Reason:    reason,
					Source:    ev.Details,
					Count:     1,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				p.db.Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "owner_id"}, {Name: "address"}},
					DoUpdates: clause.Assignments(map[string]any{
						"count":      gorm.Expr("count + 1"),
						"reason":     reason,
						"source":     ev.Details,
						"updated_at": time.Now(),
					}),
				}).Create(&item)
			}
		}

		alertText := fmt.Sprintf("⚠️ Email reputation event: Mailbox %s experienced a %s event. Details: %s", mb.Address, ev.Event, ev.Details)
		var channels []models.NotificationChannel
		p.db.Where("owner_id = ? AND enabled = ?", mb.OrgID, true).Find(&channels)
		if len(channels) > 0 {
			safego.Go("mail.bounce-notify", func() {
				ctxCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				for _, ch := range channels {
					if p.notify != nil {
						_ = p.notify(ctxCtx, ch.Type, ch.Config, alertText)
					}
				}
			})
		}
	}

	return &EmailBounceWebhookOutput{
		Body: map[string]any{"ok": true, "processed": processedCount},
	}, nil
}

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

// ownsMailHost reports whether addr's domain is one of orgID's own enabled mail
// hosts (the apex Name when a domain lists none explicitly).
//
// This is the strict form, used where a mailbox is created without an operator
// deciding — see resolveMailbox's catch-all branch.
func (p *Plugin) ownsMailHost(orgID uint, addr string) bool {
	if orgID == 0 || p.db == nil {
		return false
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return false
	}
	normHost := dns.NormalizeHost(addr[at+1:])
	if normHost == "" {
		return false
	}
	var doms []dns.Domain
	if err := p.db.Where("owner_id = ? AND for_mail = ?", orgID, true).Find(&doms).Error; err != nil {
		return false
	}
	for _, d := range doms {
		hosts := d.EffectiveMailHosts()
		if len(hosts) == 0 {
			if dns.NormalizeHost(d.Name) == normHost {
				return true
			}
			continue
		}
		for _, mh := range hosts {
			if dns.NormalizeHost(mh) == normHost {
				return true
			}
		}
	}
	return false
}

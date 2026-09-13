package mail

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/safego"
	"github.com/octarq-org/octarq/internal/usagemetric"
	"github.com/octarq-org/octarq/plugin"
)

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
		log.Printf("inbound: dropped message for unmanaged recipient %q in org %d", to, orgID)
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

	if msgID != "" {
		var existing Email
		if p.db.Where("mailbox_id = ? AND message_id = ?", mb.ID, msgID).First(&existing).Error == nil {
			return &InboundOutput{Body: map[string]any{"ok": true, "stored": true, "id": existing.ID, "duplicate": true}}, nil
		}
	}

	unsubURL := ExtractUnsubscribeURL(raw)
	p.upsertContact(mb.OrgID, from)

	e := Email{
		MailboxID:      mb.ID,
		MessageID:      msgID,
		FromAddr:       from,
		ToAddr:         to,
		Subject:        subject,
		Text:           textBody,
		HTML:           htmlBody,
		Folder:         "inbox",
		UnsubscribeURL: unsubURL,
		Attachments:    att,
		ReceivedAt:     receivedAt,
		AuthSPF:        spf,
		AuthDKIM:       dkim,
		AuthDMARC:      dmarc,
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

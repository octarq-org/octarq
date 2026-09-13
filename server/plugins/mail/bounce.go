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
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/safego"
	"github.com/octarq-org/octarq/plugin/safehttp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// snsConfirmClient fetches the attacker-influenced SNS SubscribeURL. The host
// allowlist below runs before it, but a hostname check alone can be rebound
// between the check and the dial - this client re-validates the resolved IP at
// dial time.
var snsConfirmClient = safehttp.NewClient(10 * time.Second)

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
					// over https, and fetch through snsConfirmClient (blocks
					// private/loopback/metadata IPs at dial time). SubscribeURL
					// is attacker-influenced, so it must never be fetched blindly.
					if !isAWSSNSURL(subURL) {
						log.Printf("bounce: refusing SNS SubscribeURL with non-AWS host: %s", subURL)
						return nil, huma.Error400BadRequest("invalid SubscribeURL")
					}
					safego.Go("mail.sns-confirm", func() {
						resp, err := safehttp.Get(context.Background(), snsConfirmClient, subURL, "")
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

	var emails []string
	for _, ev := range events {
		emails = append(emails, strings.ToLower(ev.Email))
	}

	var mailboxes []Mailbox
	if err := p.db.Where("address IN ? AND owner_id = ?", emails, org.ID).Find(&mailboxes).Error; err != nil {
		return nil, huma.Error500InternalServerError("database error querying mailboxes")
	}

	mbMap := make(map[string]*Mailbox, len(mailboxes))
	for i := range mailboxes {
		mbMap[strings.ToLower(mailboxes[i].Address)] = &mailboxes[i]
	}

	var channels []models.NotificationChannel
	p.db.Where("owner_id = ? AND enabled = ?", org.ID, true).Find(&channels)

	var auditLogs []models.AuditLog

	type suppressionKey struct {
		address string
	}
	// Address -> count, reason, source
	type suppressionData struct {
		count  int
		reason string
		source string
	}
	suppressions := make(map[suppressionKey]*suppressionData)

	var alertTexts []string

	for _, ev := range events {
		mb, ok := mbMap[strings.ToLower(ev.Email)]
		if !ok {
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

		auditLogs = append(auditLogs, models.AuditLog{
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
				key := suppressionKey{address: addr}
				if sData, exists := suppressions[key]; exists {
					sData.count++
					sData.reason = reason
					sData.source = ev.Details
				} else {
					suppressions[key] = &suppressionData{
						count:  1,
						reason: reason,
						source: ev.Details,
					}
				}
			}
		}

		alertTexts = append(alertTexts, fmt.Sprintf("⚠️ Email reputation event: Mailbox %s experienced a %s event. Details: %s", mb.Address, ev.Event, ev.Details))
	}

	if len(auditLogs) > 0 {
		p.db.Create(&auditLogs)
	}

	if len(suppressions) > 0 {
		var supItems []MailSuppression
		for key, data := range suppressions {
			supItems = append(supItems, MailSuppression{
				OrgID:     org.ID,
				Address:   key.address,
				Reason:    data.reason,
				Source:    data.source,
				Count:     data.count,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			})
		}

		p.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "owner_id"}, {Name: "address"}},
			DoUpdates: clause.Assignments(map[string]any{
				"count":      gorm.Expr("mail_suppressions.count + excluded.count"),
				"reason":     clause.Column{Table: "excluded", Name: "reason"},
				"source":     clause.Column{Table: "excluded", Name: "source"},
				"updated_at": clause.Column{Table: "excluded", Name: "updated_at"},
			}),
		}).Create(&supItems)
	}

	if len(channels) > 0 && len(alertTexts) > 0 {
		for _, alertText := range alertTexts {
			// Capture variables for goroutine
			alertText := alertText
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

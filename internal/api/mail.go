package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/octarq-org/octarq/plugins/dns"
	mailmodels "github.com/octarq-org/octarq/plugins/mail"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
	"github.com/octarq-org/octarq/plugin"
)

// --- mailboxes ---

type mailboxDTO struct {
	Address string `json:"address,omitempty"`
	Note    string `json:"note,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

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

func (h *Handler) inbound(ctx context.Context, input *InboundInput) (*InboundOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	// The {orgSlug} path segment names the tenant: a shared inbound host can't be
	// told apart by Host, so delivery is confined to this org's mailboxes.
	var org models.Org
	if h.db.Where("slug = ?", input.OrgSlug).First(&org).Error != nil {
		return nil, huma.Error404NotFound("unknown org")
	}
	// Auth is the org's per-tenant token, carried in the path so the Cloudflare
	// worker needs only this one URL and no custom header.
	if org.InboundToken == "" || !secureEqual(input.Token, org.InboundToken) {
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

	mb, ok := h.resolveMailbox(org.ID, to)
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
	e := mailmodels.Email{
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
			ctxCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, ch := range channels {
				_ = notify.Send(ctxCtx, ch.Type, ch.Config, text)
			}
		}()
	}

	return &InboundOutput{Body: map[string]any{"ok": true, "stored": true, "id": e.ID}}, nil
}

// mailHostDisabled reports whether host is listed as a mail host on some domain
// but every such listing is disabled (so mail to it should be dropped).
func (h *Handler) mailHostDisabled(host string) bool {
	var doms []dns.Domain
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
func (h *Handler) resolveMailbox(orgID uint, addr string) (*mailmodels.Mailbox, bool) {
	if addr == "" {
		return nil, false
	}
	// Drop mail to a temporarily disabled mail host, even for existing mailboxes.
	if at := strings.LastIndex(addr, "@"); at >= 0 && h.mailHostDisabled(addr[at+1:]) {
		return nil, false
	}
	var mb mailmodels.Mailbox
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
	var doms []dns.Domain
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
	mb = mailmodels.Mailbox{OrgID: orgID, Address: addr, Enabled: true, Note: "auto (catch-all)"}
	if err := h.db.Create(&mb).Error; err != nil {
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
func (h *Handler) emailBounceWebhook(ctx context.Context, input *EmailBounceWebhookInput) (*EmailBounceWebhookOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	// Authenticate + scope by tenant (same scheme as inbound): the {orgSlug}
	// names the org, the {token} must match its per-org secret. Without this a
	// forged POST could spam an org's notification channels and audit log.
	var org models.Org
	if h.db.Where("slug = ?", input.OrgSlug).First(&org).Error != nil {
		return nil, huma.Error404NotFound("unknown org")
	}
	if org.InboundToken == "" || !secureEqual(input.Token, org.InboundToken) {
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
						resp, err := safeGet(context.Background(), subURL)
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
		var mb mailmodels.Mailbox
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
				ctxCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				for _, ch := range chans {
					_ = notify.Send(ctxCtx, ch.Type, ch.Config, txt)
				}
			}(channels, alertText)
		}
	}

	return &EmailBounceWebhookOutput{
		Body: map[string]any{"ok": true, "processed": processedCount},
	}, nil
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

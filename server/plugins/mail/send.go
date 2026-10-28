package mail

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/mail"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/usagemetric"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/dns"
	"github.com/octarq-org/octarq/server/plugins/links"
)

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
		return nil, huma.Error500InternalServerError("An unexpected error occurred while processing the request context.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if len(input.Body.To) == 0 {
		return nil, huma.Error400BadRequest("Recipient email address (to) is required.")
	}
	orgID := p.orgID(r)
	suppressedAddr, err := p.anySuppressed(orgID, input.Body.To)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not check the suppression list. Try again later.")
	}
	if suppressedAddr != "" {
		return nil, huma.Error400BadRequest(fmt.Sprintf("Recipient address %s is on the suppression list. Remove it from suppressions before sending.", suppressedAddr))
	}

	// Rate-limit outbound mail per org so a leaked API token or a runaway client
	// can't burn the sending domain's / relay IP's reputation into an RBL.
	orgKey := fmt.Sprintf("org:%d", p.orgID(r))
	if !p.sendLimiter.allow(orgKey) {
		return nil, huma.Error429TooManyRequests("Outbound send rate limit exceeded (maximum 100 per hour). Wait before trying again.")
	}

	if input.Body.SMTPSenderID == 0 {
		return nil, huma.Error400BadRequest("No SMTP sender selected. Choose a configured SMTP sender.")
	}

	var sender mail.Sender
	var senderFrom string

	// 1. Try resolving sender via Host SPI (Centralized Integrations Hub)
	if p.host != nil {
		cred, err := p.host.LookupCredential(ctx, orgID, plugin.CredentialQuery{
			ID:           input.Body.SMTPSenderID,
			ProviderType: "communication",
		})
		if err == nil && cred != nil {
			s, fromAddr, sErr := p.senderFromCredential(cred)
			if sErr == nil && s != nil {
				sender = s
				senderFrom = fromAddr
			}
		}
	}

	// 2. Fallback to local SMTPSender table
	if sender == nil {
		var s SMTPSender
		if err := p.db.Where("id = ? AND owner_id = ?", input.Body.SMTPSenderID, orgID).First(&s).Error; err != nil {
			return nil, huma.Error400BadRequest("Configured SMTP sender could not be found for this workspace.")
		}
		if p.decrypt == nil {
			return nil, huma.Error500InternalServerError("Decryption service is unavailable. Verify encryption configuration.")
		}
		pass, err := p.decrypt(s.Pass)
		if err != nil {
			return nil, huma.Error500InternalServerError("Could not decrypt SMTP sender credentials. Update sender credentials.")
		}
		sender = mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)
		senderFrom = s.FromEmail
	}

	// Force the From header to the sender's verified address — never trust a
	// client-supplied From, which would let a caller spoof arbitrary senders
	// through the relay.
	msg := input.Body.Message
	msg.From = senderFrom

	// Metered consumption: outbound mail is the anti-abuse-critical metric on
	// the hosted build (a Free org sending mail would burn the sending domain
	// onto a blocklist that would take down paying tenants too), so ask the
	// (hosted-only) quota checker whether this org may send n more. Self-hosted
	// has no checker and this always passes there. The check runs before the
	// first real side effect — actually handing the message to the SMTP relay —
	// so a refusal sends nothing.
	if err := plugin.CheckQuota(p.ctx, ctx, orgID, "mailOutPerMonth", int64(len(input.Body.To))); err != nil {
		if errors.Is(err, plugin.ErrQuotaUnavailable) {
			return nil, huma.Error402PaymentRequired("Outbound mail is not included in this plan. Upgrade your plan to unlock email sending.")
		}
		return nil, huma.Error429TooManyRequests("Outbound mail quota exceeded for this workspace. Upgrade your plan or wait for the quota to reset.")
	}

	if input.Body.TrackLinks {
		p.wrapLinksInEmail(r, &msg)
	}

	if err := sender.Send(msg); err != nil {
		if p.publishEvent != nil {
			p.publishEvent(p.orgID(r), "email.send_failed", map[string]any{"to": msg.To, "subject": msg.Subject, "error": err.Error()})
		}
		return nil, huma.Error400BadRequest("Could not deliver email: " + err.Error())
	}
	if p.recordUsage != nil {
		p.recordUsage(p.orgID(r), usagemetric.MailOut, int64(len(msg.To)))
	}
	p.upsertContacts(orgID, input.Body.To)
	p.recordSentEmail(orgID, senderFrom, strings.Join(input.Body.To, ", "), msg.Subject, msg.Text, msg.HTML)
	p.sendLimiter.recordFailure(orgKey) // count this send against the per-org cap
	return &SendEmailOutput{Body: map[string]bool{"ok": true}}, nil
}

func (p *Plugin) wrapLinksInEmail(r *http.Request, msg *mail.Message) {
	orgID := p.orgID(r)

	var doms []dns.Domain
	p.db.Where("owner_id = ? AND for_link = ?", orgID, true).Find(&doms)
	shortHost := r.Host
	if len(doms) > 0 {
		shortHost = doms[0].Name
	}
	if idx := strings.IndexByte(shortHost, ':'); idx >= 0 {
		shortHost = shortHost[:idx]
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	reLink := regexp.MustCompile(`https?://[a-zA-Z0-9.\-_~%#?&=/]+`)

	urlMap := make(map[string]string)

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

			if cached, ok := urlMap[cleanURL]; ok {
				return cached + suffix
			}

			var (
				slug  string
				found bool
			)
			for i := 0; i < 5; i++ {
				candidate := models.RandomSlug(6)
				if !p.isReservedSlug(candidate) {
					var count int64
					p.db.Model(&links.Link{}).Where("slug = ?", candidate).Count(&count)
					if count == 0 {
						slug = candidate
						found = true
						break
					}
				}
			}
			if !found {
				return rawURL
			}

			link := links.Link{
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

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	netmail "net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/mail"
	"github.com/octarq-org/octarq/server/internal/usagemetric"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func (p *Plugin) getStorageProvider() (plugin.StorageProvider, error) {
	var sp plugin.StorageProvider
	if p.ctx != nil {
		if found, ok := plugin.LookupAs[plugin.StorageProvider](p.ctx, plugin.ServiceMailStorageProvider); ok && found != nil {
			sp = found
		}
	}

	backend := p.getBackendConfig()
	if backend != "" && backend != "database" && backend != "db" {
		if _, isDB := sp.(*DBStorageProvider); sp != nil && !isDB {
			return sp, nil
		}
		return nil, fmt.Errorf("mail storage provider %q requires Pro edition", backend)
	}

	if sp != nil {
		return sp, nil
	}
	return NewDBStorageProvider(p.db), nil
}

// getBackendConfig resolves which storage backend to use. The Pro mailstorage
// module owns the authoritative configuration table and pushes the runtime key
// here via SetGlobalSetting; absent a value the database backend — the only one
// OSS ships — applies.
func (p *Plugin) getBackendConfig() string {
	if p.getGlobalSetting != nil {
		if val := strings.TrimSpace(p.getGlobalSetting("mail_storage_backend")); val != "" {
			return val
		}
	}
	return "database"
}

func (p *Plugin) isSuppressed(orgID uint, addr string) bool {
	if addr == "" || orgID == 0 {
		return false
	}
	normAddr := strings.ToLower(strings.TrimSpace(addr))
	if parsed, err := netmail.ParseAddress(normAddr); err == nil && parsed.Address != "" {
		normAddr = strings.ToLower(parsed.Address)
	}
	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return false
	}
	var count int64
	tdb.Model(&MailSuppression{}).Where("address = ?", normAddr).Count(&count)
	return count > 0
}

func (p *Plugin) anySuppressed(orgID uint, addrs []string) (string, error) {
	if len(addrs) == 0 || orgID == 0 {
		return "", nil
	}
	normAddrs := make([]string, 0, len(addrs))

	for _, addr := range addrs {
		normAddr := strings.ToLower(strings.TrimSpace(addr))
		if parsed, err := netmail.ParseAddress(normAddr); err == nil && parsed.Address != "" {
			normAddr = strings.ToLower(parsed.Address)
		}
		if normAddr != "" {
			normAddrs = append(normAddrs, normAddr)
		}
	}

	if len(normAddrs) == 0 {
		return "", nil
	}

	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return "", nil
	}
	var suppressed []string
	err := tdb.Model(&MailSuppression{}).Where("address IN ?", normAddrs).Pluck("address", &suppressed).Error
	if err != nil {
		return "", err
	}

	if len(suppressed) > 0 {
		return suppressed[0], nil
	}
	return "", nil
}

// senderFromCredential builds a mail.Sender and sender from-address from a
// decrypted Credential (supporting SMTP relays and Resend).
func (p *Plugin) senderFromCredential(cred *plugin.Credential) (mail.Sender, string, error) {
	if cred == nil || len(cred.ConfigJSON) == 0 {
		return nil, "", errors.New("empty credential configuration")
	}

	var m struct {
		Host      string `json:"host"`
		SMTPHost  string `json:"smtpHost"`
		Port      int    `json:"port"`
		User      string `json:"user"`
		APIKey    string `json:"apiKey"`
		Pass      string `json:"pass"`
		SecretKey string `json:"secretKey"`
		FromEmail string `json:"fromEmail"`
	}
	if err := json.Unmarshal(cred.ConfigJSON, &m); err != nil {
		return nil, "", fmt.Errorf("malformed credential configuration: %w", err)
	}

	from := strings.TrimSpace(m.FromEmail)
	if from == "" {
		from = "no-reply@octarq.local"
	}

	if cred.Provider == "resend" {
		apiKey := m.APIKey
		if apiKey == "" {
			apiKey = m.SecretKey
		}
		if apiKey == "" {
			return nil, "", errors.New("resend API key is required")
		}
		return mail.NewResendSender(apiKey, from), from, nil
	}

	// Default to SMTP
	host := m.SMTPHost
	if host == "" {
		host = m.Host
	}
	if host == "" {
		return nil, "", errors.New("smtp host is required")
	}
	port := m.Port
	if port == 0 {
		port = 587
	}
	user := m.User
	if user == "" {
		user = m.APIKey
	}
	pass := m.Pass
	if pass == "" {
		pass = m.SecretKey
	}

	return mail.NewCustomSender(host, fmt.Sprint(port), user, pass, from), from, nil
}

// mailReady reports whether the instance's SYSTEM sender is available: at
// least one SMTP sender exists somewhere on the instance, which is exactly
// when sendSystemMail (via the mail.send.system service) can deliver.
// Consumed through the mail.ready service by the registration verification
// gate and both readiness reports: "plugin mounted" is not the same question
// as "can this instance deliver a system message".
func (p *Plugin) mailReady() bool {
	var n int64
	p.db.Model(&SMTPSender{}).Count(&n)
	if n > 0 {
		return true
	}
	if p.host != nil {
		for _, provider := range []string{"smtp", "resend"} {
			cred, err := p.host.LookupCredential(context.Background(), 0, plugin.CredentialQuery{
				ProviderType: "communication",
				Provider:     provider,
			})
			if err == nil && cred != nil {
				return true
			}
		}
	}
	return false
}

// systemSender resolves the instance's system sender: the one explicitly
// selected in instance settings (mail_system_sender_id). Pre-v1.0 fail-closed
// invariants strictly prohibit falling back to lowest-id or arbitrary tenant senders.
func (p *Plugin) systemSender() (*SMTPSender, error) {
	if p.getGlobalSetting != nil {
		if idStr := strings.TrimSpace(p.getGlobalSetting("mail_system_sender_id")); idStr != "" {
			if id, err := strconv.ParseUint(idStr, 10, 64); err == nil && id != 0 {
				var s SMTPSender
				if err := p.db.First(&s, id).Error; err == nil {
					return &s, nil
				}
				return nil, fmt.Errorf("configured system SMTP sender %d not found: %w", id, gorm.ErrRecordNotFound)
			}
			return nil, fmt.Errorf("invalid mail_system_sender_id setting %q", idStr)
		}
	}
	return nil, fmt.Errorf("no SMTP sender configured on this instance; configure mail_system_sender_id in system settings")
}

// deliverVia sends one message through the given sender record, handling
// password decryption, usage metering and the email.send_failed event. Shared
// by system mail, org mail and the per-sender connectivity test so the three
// cannot drift on how a send is metered or reported.
func (p *Plugin) deliverVia(s *SMTPSender, to, subject, htmlBody, textBody string) error {
	pass, err := p.decrypt(s.Pass)
	if err != nil {
		return err
	}
	sender := mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)
	if err := sender.Send(mail.Message{From: s.FromEmail, To: []string{to}, Subject: subject, HTML: htmlBody, Text: textBody}); err != nil {
		if p.publishEvent != nil {
			p.publishEvent(s.OrgID, "email.send_failed", map[string]any{"to": []string{to}, "subject": subject, "error": err.Error()})
		}
		return err
	}
	if p.recordUsage != nil {
		p.recordUsage(s.OrgID, usagemetric.MailOut, 1)
	}
	return nil
}

// sendSystemMail delivers an instance-level system message (verification,
// password reset, invite) through the system sender resolved by systemSender.
// Unlike sendMail it has no orgID: these flows must work for recipients with
// no membership yet. Usage and failure events are attributed to the sender's
// owning workspace so metering and webhooks keep a real org id.
func (p *Plugin) sendSystemMail(to, subject, htmlBody, textBody string) error {
	s, err := p.systemSender()
	if err != nil {
		return err
	}
	return p.deliverVia(s, to, subject, htmlBody, textBody)
}

func (p *Plugin) sendMail(orgID uint, to, subject, htmlBody, textBody string) error {
	if p.isSuppressed(orgID, to) {
		return fmt.Errorf("recipient address %s is in suppression list", to)
	}

	// 1. Try resolving a mail-capable sender via Host SPI (Centralized
	// Integrations Hub). Query the mail providers explicitly: a bare
	// provider_type=communication lookup can return a Telegram or Slack
	// integration, which cannot send email.
	if p.host != nil {
		for _, provider := range []string{"smtp", "resend"} {
			cred, err := p.host.LookupCredential(context.Background(), orgID, plugin.CredentialQuery{
				ProviderType: "communication",
				Provider:     provider,
			})
			if err != nil {
				if errors.Is(err, plugin.ErrCredentialNotFound) {
					continue
				}
				return fmt.Errorf("resolve %s integration for org %d: %w", provider, orgID, err)
			}
			if cred == nil {
				continue
			}
			sender, from, sErr := p.senderFromCredential(cred)
			if sErr != nil {
				return fmt.Errorf("invalid %s integration for org %d: %w", provider, orgID, sErr)
			}
			if err := sender.Send(mail.Message{From: from, To: []string{to}, Subject: subject, HTML: htmlBody, Text: textBody}); err != nil {
				if p.publishEvent != nil {
					p.publishEvent(orgID, "email.send_failed", map[string]any{"to": []string{to}, "subject": subject, "error": err.Error()})
				}
				return err
			}
			if p.recordUsage != nil {
				p.recordUsage(orgID, usagemetric.MailOut, 1)
			}
			return nil
		}
	}

	// 2. Fallback to local SMTPSender table
	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return fmt.Errorf("database unavailable for org %d", orgID)
	}
	var s SMTPSender
	if err := tdb.Order("id").First(&s).Error; err != nil {
		return fmt.Errorf("no SMTP sender configured for org %d", orgID)
	}
	return p.deliverVia(&s, to, subject, htmlBody, textBody)
}

func (p *Plugin) recordSentEmail(orgID uint, from, to, subject, text, html string) {
	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return
	}
	var mb Mailbox
	if err := tdb.Where("address = ?", from).First(&mb).Error; err != nil {
		if err := tdb.First(&mb).Error; err != nil {
			mb = Mailbox{OrgID: orgID, Address: from, Enabled: true, Note: "outbound"}
			if err := tdb.Create(&mb).Error; err != nil {
				log.Printf("mail: failed to auto-create outbound mailbox for org %d: %v", orgID, err)
			}
		}
	}
	if mb.ID != 0 {
		sentEmail := Email{
			MailboxID:  mb.ID,
			FromAddr:   from,
			ToAddr:     to,
			Subject:    subject,
			Text:       text,
			HTML:       html,
			Folder:     "sent",
			Read:       true,
			ReceivedAt: time.Now(),
		}
		if err := p.db.Create(&sentEmail).Error; err != nil {
			log.Printf("mail: failed to persist sent email record for mailbox %d: %v", mb.ID, err)
		}
	}
}

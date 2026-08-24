package mail

import (
	"fmt"
	netmail "net/mail"
	"strconv"
	"strings"

	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/usagemetric"
	"github.com/octarq-org/octarq/plugin"
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
	var count int64
	p.db.Model(&MailSuppression{}).Where("owner_id = ? AND address = ?", orgID, normAddr).Count(&count)
	return count > 0
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
	return n > 0
}

// systemSender resolves the instance's system sender: the one selected in
// instance settings (mail_system_sender_id) when present and still existing,
// otherwise the lowest-id sender on the instance (deterministic fallback that
// also covers a stale reference to a deleted sender). Returns an explicit
// error when no sender exists at all.
func (p *Plugin) systemSender() (*SMTPSender, error) {
	if p.getGlobalSetting != nil {
		if idStr := strings.TrimSpace(p.getGlobalSetting("mail_system_sender_id")); idStr != "" {
			if id, err := strconv.ParseUint(idStr, 10, 64); err == nil && id != 0 {
				var s SMTPSender
				if err := p.db.First(&s, id).Error; err == nil {
					return &s, nil
				}
			}
		}
	}
	var s SMTPSender
	if err := p.db.Order("id ASC").First(&s).Error; err != nil {
		return nil, fmt.Errorf("no SMTP sender configured on this instance; system email cannot be sent. Configure an SMTP sender (Mail → SMTP senders) or mount a plugin providing mail.send")
	}
	return &s, nil
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

func (p *Plugin) sendMail(orgID uint, to, subject, htmlBody, textBody string) error {
	if p.isSuppressed(orgID, to) {
		return fmt.Errorf("recipient address %s is in suppression list", to)
	}
	var s SMTPSender
	if err := p.db.Where("owner_id = ?", orgID).Order("id").First(&s).Error; err != nil {
		return fmt.Errorf("no SMTP sender configured for org %d", orgID)
	}
	pass, err := p.decrypt(s.Pass)
	if err != nil {
		return err
	}
	sender := mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)
	if err := sender.Send(mail.Message{From: s.FromEmail, To: []string{to}, Subject: subject, HTML: htmlBody, Text: textBody}); err != nil {
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

package mail

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

func (p *Plugin) purge(orgID uint) error {
	ctx := context.Background()
	ctx = plugin.WithOrgID(ctx, orgID)
	mailboxIDs := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)

	storageProv, spErr := p.getStorageProvider()
	if spErr != nil {
		log.Printf("mail purge: storage provider unavailable (%v); deleting database blobs only", spErr)
	}
	dbProv := NewDBStorageProvider(p.db)
	delCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	for {
		var emails []Email
		if err := p.db.Select("id", "storage_key").Where("mailbox_id IN (?)", mailboxIDs).Limit(2000).Find(&emails).Error; err != nil {
			log.Printf("mail purge: failed to query emails for org %d: %v", orgID, err)
			break
		}
		if len(emails) == 0 {
			break
		}

		for _, e := range emails {
			key := e.StorageKey
			if key == "" {
				key = fmt.Sprintf("mail/%d/%d.eml", orgID, e.ID)
			}
			if storageProv != nil {
				if err := storageProv.Delete(delCtx, key); err != nil {
					log.Printf("mail purge: failed to delete storage blob %q: %v", key, err)
				}
			}
			if err := dbProv.Delete(delCtx, key); err != nil {
				log.Printf("mail purge: failed to delete database blob %q: %v", key, err)
			}
		}

		var ids []uint
		for _, e := range emails {
			ids = append(ids, e.ID)
		}
		if err := p.db.Where("id IN (?)", ids).Delete(&Email{}).Error; err != nil {
			log.Printf("mail purge: failed to delete email records for org %d: %v", orgID, err)
			break
		}
	}

	p.db.Where("owner_id = ?", orgID).Delete(&Mailbox{})
	p.db.Where("owner_id = ?", orgID).Delete(&SMTPSender{})
	p.db.Where("owner_id = ?", orgID).Delete(&MailSuppression{})
	return nil
}

func (p *Plugin) exportData(orgID uint) map[string]any {
	var mailboxes []Mailbox
	var emails []Email
	var smtp []SMTPSender
	var suppressions []MailSuppression
	p.db.Where("owner_id = ?", orgID).Find(&mailboxes)
	mailboxIDs := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)
	p.db.Where("mailbox_id IN (?)", mailboxIDs).Find(&emails)
	p.db.Where("owner_id = ?", orgID).Find(&smtp)
	p.db.Where("owner_id = ?", orgID).Find(&suppressions)
	return map[string]any{
		"mailboxes":    mailboxes,
		"emails":       emails,
		"smtpSenders":  smtp,
		"suppressions": suppressions,
	}
}

var htmlTagRe = regexp.MustCompile(`(?s)<style.*?</style>|<script.*?</script>|<[^>]*>`)

func (p *Plugin) getEmailForSummarize(orgID uint, id uint) (from, subject, body string, ok bool) {
	var count int64
	p.db.Model(&Email{}).
		Joins("JOIN mailboxes ON mailboxes.id = emails.mailbox_id AND mailboxes.owner_id = ?", orgID).
		Where("emails.id = ?", id).Count(&count)
	if count == 0 {
		return "", "", "", false
	}
	var e Email
	if p.db.First(&e, id).Error != nil {
		return "", "", "", false
	}
	b := e.Text
	if strings.TrimSpace(b) == "" {
		b = htmlTagRe.ReplaceAllString(e.HTML, " ")
	}
	return e.FromAddr, e.Subject, b, true
}

func (p *Plugin) overview(orgID uint, includeBot bool) map[string]any {
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)
	count := func(model any, conds ...any) int64 {
		var n int64
		q := p.db.Model(model).Where("owner_id = ?", orgID)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}
	emailCount := func(conds ...any) int64 {
		var n int64
		q := p.db.Model(&Email{}).Where("mailbox_id IN (?)", orgMailboxes)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}
	type recentEmail struct {
		ID         uint      `json:"id"`
		FromAddr   string    `json:"from"`
		Subject    string    `json:"subject"`
		Read       bool      `json:"read"`
		ReceivedAt time.Time `json:"receivedAt"`
	}
	var recent []recentEmail
	p.db.Model(&Email{}).
		Select("id, from_addr, subject, read, received_at").
		Where("mailbox_id IN (?)", orgMailboxes).
		Order("received_at DESC").Limit(6).Scan(&recent)

	return map[string]any{
		"mailboxes":    count(&Mailbox{}),
		"emails":       emailCount(),
		"unread":       emailCount("read = ?", false),
		"recentEmails": recent,
	}
}

var builtinReservedSlugs = map[string]bool{
	"admin": true, "api": true, "assets": true, "portal": true,
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (p *Plugin) isReservedSlug(slug string) bool {
	slug = strings.ToLower(slug)
	if builtinReservedSlugs[slug] {
		return true
	}
	if p.getGlobalSetting != nil {
		for _, res := range splitList(p.getGlobalSetting("reserved_slugs")) {
			if res == slug {
				return true
			}
		}
	}
	return false
}

// hasRole reports whether the caller holds at least the given workspace role.
//
// A host that never wired RequireRole is refused rather than waved through. The
// gate protects destructive and credential-bearing operations, so "the host did
// not tell us who this is" has to mean no, not yes — an unwired seam would
// otherwise silently disable every role check in this plugin.
func (p *Plugin) hasRole(r *http.Request, min string) bool {
	if p.requireRole == nil {
		return false
	}
	return p.requireRole(r, min)
}

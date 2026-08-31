package mail

import (
	"context"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// upsertContact maintains address interaction history for auto-completion and tracking.
func (p *Plugin) upsertContact(orgID uint, rawAddr string) {
	if orgID == 0 || rawAddr == "" {
		return
	}
	parsed, err := netmail.ParseAddress(rawAddr)
	var addr, name string
	if err == nil && parsed.Address != "" {
		addr = strings.ToLower(strings.TrimSpace(parsed.Address))
		name = strings.TrimSpace(parsed.Name)
	} else {
		addr = strings.ToLower(strings.TrimSpace(rawAddr))
		addr = strings.Trim(addr, "<>")
	}
	if addr == "" || !strings.Contains(addr, "@") {
		return
	}

	var contact MailContact
	err = p.db.Where("owner_id = ? AND address = ?", orgID, addr).First(&contact).Error
	now := time.Now()
	if err == nil {
		updates := map[string]any{
			"interaction_count": gorm.Expr("interaction_count + 1"),
			"last_seen_at":      now,
		}
		if contact.Name == "" && name != "" {
			updates["name"] = name
		}
		p.db.Model(&contact).Updates(updates)
	} else {
		contact = MailContact{
			OrgID:            orgID,
			Address:          addr,
			Name:             name,
			InteractionCount: 1,
			LastSeenAt:       now,
		}
		_ = p.db.Create(&contact).Error
	}
}

type ListContactsInput struct {
	Ctx    huma.Context `hidden:"true"`
	Q      string       `query:"q" doc:"Search query"`
	Query  string       `query:"query" doc:"Search query alias"`
	Limit  int          `query:"limit"`
	Offset int          `query:"offset"`
}

func (i *ListContactsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListContactsOutput struct {
	Body []MailContact
}

func (p *Plugin) listContacts(ctx context.Context, input *ListContactsInput) (*ListContactsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	q := p.db.Model(&MailContact{}).Where("owner_id = ?", orgID)
	searchTerm := strings.TrimSpace(input.Q)
	if searchTerm == "" {
		searchTerm = strings.TrimSpace(input.Query)
	}
	if searchTerm != "" {
		like := "%" + searchTerm + "%"
		q = q.Where("address LIKE ? OR name LIKE ?", like, like)
	}

	limit := plugin.PageLimit(input.Limit, 50, 500)
	offset := plugin.PageOffset(input.Offset)
	q = q.Order("interaction_count DESC, last_seen_at DESC").Limit(limit).Offset(offset)

	var contacts []MailContact
	if err := q.Find(&contacts).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to query contacts")
	}
	return &ListContactsOutput{Body: contacts}, nil
}

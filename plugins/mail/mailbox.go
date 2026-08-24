package mail

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

type cntRow struct {
	MailboxID uint
	Cnt       int64
}

type ListMailboxesInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListMailboxesInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListMailboxesOutput struct {
	Body []Mailbox
}

func (p *Plugin) listMailboxes(ctx context.Context, input *ListMailboxesInput) (*ListMailboxesOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var boxes []Mailbox
	p.orgDB(r).Order("created_at DESC").Find(&boxes)
	if len(boxes) > 0 {
		ids := make([]uint, 0, len(boxes))
		for _, b := range boxes {
			ids = append(ids, b.ID)
		}
		var rows []cntRow
		p.db.Model(&Email{}).Select("mailbox_id, count(*) as cnt").Where("mailbox_id IN ? AND read = ?", ids, false).Group("mailbox_id").Find(&rows)
		m := make(map[uint]int64, len(rows))
		for _, r2 := range rows {
			m[r2.MailboxID] = r2.Cnt
		}
		for i := range boxes {
			boxes[i].Unread = m[boxes[i].ID]
		}
	}
	return &ListMailboxesOutput{Body: boxes}, nil
}

type mailboxDTO struct {
	Address string `json:"address,omitempty"`
	Note    string `json:"note,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type CreateMailboxInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body mailboxDTO
}

func (i *CreateMailboxInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateMailboxOutput struct {
	Body Mailbox
}

func (p *Plugin) createMailbox(ctx context.Context, input *CreateMailboxInput) (*CreateMailboxOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to create mailbox")
	}
	addr := strings.TrimSpace(strings.ToLower(input.Body.Address))
	if !strings.Contains(addr, "@") {
		return nil, huma.Error400BadRequest("address must be a full email")
	}
	if !p.mailAddressDomainNotAnotherTenants(p.orgID(r), addr) {
		return nil, huma.Error403Forbidden("address domain is a mail host of another workspace")
	}
	enabled := true
	if input.Body.Enabled != nil {
		enabled = *input.Body.Enabled
	}
	mb := Mailbox{
		OrgID:   p.orgID(r),
		Address: addr, Note: input.Body.Note, Enabled: enabled,
	}
	if err := p.db.Create(&mb).Error; err != nil {
		return nil, huma.NewError(http.StatusConflict, "mailbox already exists")
	}
	if p.audit != nil {
		p.audit(r, "mailbox.create", "mailbox", mb.ID, map[string]any{"address": mb.Address})
	}
	return &CreateMailboxOutput{Body: mb}, nil
}

type UpdateMailboxInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body mailboxDTO
}

func (i *UpdateMailboxInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateMailboxOutput struct {
	Body Mailbox
}

func (p *Plugin) updateMailbox(ctx context.Context, input *UpdateMailboxInput) (*UpdateMailboxOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to update mailbox")
	}
	var mb Mailbox
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&mb).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	mb.Note = input.Body.Note
	if input.Body.Enabled != nil {
		mb.Enabled = *input.Body.Enabled
	}
	p.db.Save(&mb)
	meta := map[string]any{
		"note":    mb.Note,
		"enabled": mb.Enabled,
	}
	if p.audit != nil {
		p.audit(r, "mailbox.update", "mailbox", mb.ID, meta)
	}
	return &UpdateMailboxOutput{Body: mb}, nil
}

type DeleteMailboxInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteMailboxInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteMailboxOutput struct {
	Body map[string]bool
}

func (p *Plugin) deleteMailbox(ctx context.Context, input *DeleteMailboxInput) (*DeleteMailboxOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to delete mailbox")
	}
	var emails []Email
	p.db.Where("mailbox_id = ?", input.ID).Find(&emails)
	res := p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).Delete(&Mailbox{})
	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	ctx = plugin.WithOrgID(ctx, p.orgID(r))
	storageProv, _ := p.getStorageProvider()
	dbProv := NewDBStorageProvider(p.db)
	for _, e := range emails {
		key := e.StorageKey
		if key == "" {
			key = fmt.Sprintf("mail/%d/%d.eml", p.orgID(r), e.ID)
		}
		if storageProv != nil {
			_ = storageProv.Delete(ctx, key)
		}
		_ = dbProv.Delete(ctx, key)
	}
	p.db.Where("mailbox_id = ?", input.ID).Delete(&Email{})
	if p.audit != nil {
		p.audit(r, "mailbox.delete", "mailbox", input.ID, nil)
	}
	return &DeleteMailboxOutput{Body: map[string]bool{"ok": true}}, nil
}

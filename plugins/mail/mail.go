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
	for i := range boxes {
		p.db.Model(&Email{}).
			Where("mailbox_id = ? AND read = ?", boxes[i].ID, false).
			Count(&boxes[i].Unread)
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
	res := p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).Delete(&Mailbox{})
	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	p.db.Where("mailbox_id = ?", input.ID).Delete(&Email{})
	if p.audit != nil {
		p.audit(r, "mailbox.delete", "mailbox", input.ID, nil)
	}
	return &DeleteMailboxOutput{Body: map[string]bool{"ok": true}}, nil
}

type ListEmailsInput struct {
	Ctx     huma.Context `hidden:"true"`
	Mailbox string       `query:"mailbox"`
	Q       string       `query:"q"`
	Limit   int          `query:"limit"`
	Offset  int          `query:"offset"`
}

func (i *ListEmailsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListEmailsOutput struct {
	Body []Email
}

func (p *Plugin) listEmails(ctx context.Context, input *ListEmailsInput) (*ListEmailsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	q := p.db.Where("mailbox_id IN (?)", orgMailboxes).Order("received_at DESC").Omit("Raw", "HTML")
	if input.Mailbox != "" {
		q = q.Where("mailbox_id = ?", input.Mailbox)
	}
	if input.Q != "" {
		like := "%" + input.Q + "%"
		q = q.Where("subject LIKE ? OR from_addr LIKE ? OR text LIKE ? OR note LIKE ?", like, like, like, like)
	}
	limit := plugin.PageLimit(input.Limit, 50, 500)
	offset := plugin.PageOffset(input.Offset)
	q = q.Limit(limit).Offset(offset)
	var emails []Email
	q.Find(&emails)
	return &ListEmailsOutput{Body: emails}, nil
}

type GetEmailInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *GetEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetEmailOutput struct {
	Body Email
}

func (p *Plugin) getEmail(ctx context.Context, input *GetEmailInput) (*GetEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	var e Email
	if p.db.Where("id = ? AND mailbox_id IN (?)", input.ID, orgMailboxes).First(&e).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	if !e.Read {
		p.db.Model(&e).Update("read", true)
		e.Read = true
	}
	return &GetEmailOutput{Body: e}, nil
}

type UpdateEmailInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body struct {
		Read *bool   `json:"read,omitempty"`
		Note *string `json:"note,omitempty"`
	}
}

func (i *UpdateEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateEmailOutput struct {
	Body Email
}

func (p *Plugin) updateEmail(ctx context.Context, input *UpdateEmailInput) (*UpdateEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	var e Email
	if p.db.Where("id = ? AND mailbox_id IN (?)", input.ID, orgMailboxes).First(&e).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	if input.Body.Read != nil {
		e.Read = *input.Body.Read
	}
	if input.Body.Note != nil {
		e.Note = *input.Body.Note
	}
	p.db.Save(&e)
	return &UpdateEmailOutput{Body: e}, nil
}

type ReadAllEmailsInput struct {
	Ctx     huma.Context `hidden:"true"`
	Mailbox string       `query:"mailbox"`
}

func (i *ReadAllEmailsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ReadAllEmailsOutput struct {
	Body map[string]any
}

// readAllEmails marks every email read, optionally scoped to one mailbox.
func (p *Plugin) readAllEmails(ctx context.Context, input *ReadAllEmailsInput) (*ReadAllEmailsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	q := p.db.Model(&Email{}).Where("read = ? AND mailbox_id IN (?)", false, orgMailboxes)
	if input.Mailbox != "" {
		q = q.Where("mailbox_id = ?", input.Mailbox)
	}
	res := q.Update("read", true)
	return &ReadAllEmailsOutput{
		Body: map[string]any{"ok": true, "updated": res.RowsAffected},
	}, nil
}

type RawEmailInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *RawEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// rawEmail streams the original RFC822 message as a downloadable .eml file.
func (p *Plugin) rawEmail(ctx context.Context, input *RawEmailInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	ctx = plugin.WithOrgID(ctx, p.orgID(r))
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	var e Email
	if p.db.Where("id = ? AND mailbox_id IN (?)", input.ID, orgMailboxes).First(&e).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}

	rawBytes := e.Raw
	if len(rawBytes) == 0 {
		key := e.StorageKey
		if key == "" {
			var mb Mailbox
			_ = p.db.First(&mb, e.MailboxID).Error
			key = fmt.Sprintf("mail/%d/%d.eml", mb.OrgID, e.ID)
		}
		if storageProv, err := p.getStorageProvider(); err == nil {
			if data, getErr := storageProv.Get(ctx, key); getErr == nil {
				rawBytes = data
			}
		}
		if len(rawBytes) == 0 {
			dbProv := NewDBStorageProvider(p.db)
			if data, getErr := dbProv.Get(ctx, key); getErr == nil {
				rawBytes = data
			}
		}
	}

	w.Header().Set("Content-Type", "message/rfc822")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"email-%d.eml\"", e.ID))
	w.Write(rawBytes)
	return nil, nil
}

type DeleteEmailInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteEmailOutput struct {
	Body map[string]bool
}

func (p *Plugin) deleteEmail(ctx context.Context, input *DeleteEmailInput) (*DeleteEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to delete email")
	}
	ctx = plugin.WithOrgID(ctx, p.orgID(r))
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	var e Email
	if p.db.Where("id = ? AND mailbox_id IN (?)", input.ID, orgMailboxes).First(&e).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	key := e.StorageKey
	if key == "" {
		var mb Mailbox
		_ = p.db.First(&mb, e.MailboxID).Error
		key = fmt.Sprintf("mail/%d/%d.eml", mb.OrgID, e.ID)
	}
	if storageProv, err := p.getStorageProvider(); err == nil {
		_ = storageProv.Delete(ctx, key)
	}
	dbProv := NewDBStorageProvider(p.db)
	_ = dbProv.Delete(ctx, key)
	p.db.Where("id = ? AND mailbox_id IN (?)", input.ID, orgMailboxes).Delete(&Email{})
	return &DeleteEmailOutput{Body: map[string]bool{"ok": true}}, nil
}

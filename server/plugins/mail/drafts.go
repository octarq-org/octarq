package mail

import (
	"context"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

type SaveDraftInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		ID        uint   `json:"id,omitempty"`
		MailboxID uint   `json:"mailboxId,omitempty"`
		To        string `json:"to"`
		Subject   string `json:"subject"`
		Text      string `json:"text"`
		HTML      string `json:"html,omitempty"`
	}
}

func (i *SaveDraftInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type SaveDraftOutput struct {
	Body Email
}

func (p *Plugin) saveDraft(ctx context.Context, input *SaveDraftInput) (*SaveDraftOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)

	if input.Body.ID != 0 {
		var e Email
		if p.db.Where("id = ? AND mailbox_id IN (?)", input.Body.ID, orgMailboxes).First(&e).Error != nil {
			return nil, huma.Error404NotFound("draft not found")
		}
		e.ToAddr = input.Body.To
		e.Subject = input.Body.Subject
		e.Text = input.Body.Text
		if input.Body.HTML != "" {
			e.HTML = input.Body.HTML
		}
		e.Folder = "drafts"
		e.ReceivedAt = time.Now()
		if err := p.db.Save(&e).Error; err != nil {
			return nil, huma.Error500InternalServerError("failed to update draft")
		}
		return &SaveDraftOutput{Body: e}, nil
	}

	mbID := input.Body.MailboxID
	if mbID == 0 {
		var mb Mailbox
		if err := p.db.Where("owner_id = ?", orgID).First(&mb).Error; err == nil {
			mbID = mb.ID
		} else {
			mb = Mailbox{OrgID: orgID, Address: fmt.Sprintf("drafts@org%d.local", orgID), Enabled: true, Note: "drafts"}
			if err := p.db.Create(&mb).Error; err == nil {
				mbID = mb.ID
			}
		}
	} else {
		var count int64
		p.db.Model(&Mailbox{}).Where("id = ? AND owner_id = ?", mbID, orgID).Count(&count)
		if count == 0 {
			return nil, huma.Error400BadRequest("invalid mailbox id")
		}
	}

	e := Email{
		MailboxID:  mbID,
		Folder:     "drafts",
		ToAddr:     input.Body.To,
		Subject:    input.Body.Subject,
		Text:       input.Body.Text,
		HTML:       input.Body.HTML,
		Read:       true,
		ReceivedAt: time.Now(),
	}
	if err := p.db.Create(&e).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save draft")
	}
	return &SaveDraftOutput{Body: e}, nil
}

type DeleteDraftInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteDraftInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteDraftOutput struct {
	Body map[string]bool
}

func (p *Plugin) deleteDraft(ctx context.Context, input *DeleteDraftInput) (*DeleteDraftOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)
	var e Email
	if p.db.Where("id = ? AND mailbox_id IN (?)", input.ID, orgMailboxes).First(&e).Error != nil {
		return nil, huma.Error404NotFound("draft not found")
	}
	if e.Folder != "drafts" {
		return nil, huma.Error400BadRequest("not a draft")
	}
	p.db.Delete(&e)
	return &DeleteDraftOutput{Body: map[string]bool{"ok": true}}, nil
}

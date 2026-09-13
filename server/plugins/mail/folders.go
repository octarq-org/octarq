package mail

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

type UpdateEmailFolderInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body struct {
		Folder string `json:"folder" doc:"Target folder (inbox, sent, drafts, trash, spam)" enum:"inbox,sent,drafts,trash,spam"`
	}
}

func (i *UpdateEmailFolderInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateEmailFolderOutput struct {
	Body Email
}

func (p *Plugin) updateEmailFolder(ctx context.Context, input *UpdateEmailFolderInput) (*UpdateEmailFolderOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	folder := strings.ToLower(strings.TrimSpace(input.Body.Folder))
	switch folder {
	case "inbox", "sent", "drafts", "trash", "spam":
	default:
		return nil, huma.Error400BadRequest("invalid folder: must be inbox, sent, drafts, trash, or spam")
	}

	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)
	var e Email
	if p.db.Where("id = ? AND mailbox_id IN (?)", input.ID, orgMailboxes).First(&e).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}

	e.Folder = folder
	if err := p.db.Model(&e).Update("folder", folder).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to update folder")
	}
	return &UpdateEmailFolderOutput{Body: e}, nil
}

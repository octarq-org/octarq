package mail

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"gorm.io/gorm/clause"
)

type ListSuppressionsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListSuppressionsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListSuppressionsOutput struct {
	Body []MailSuppression
}

func (p *Plugin) listSuppressions(ctx context.Context, input *ListSuppressionsInput) (*ListSuppressionsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var items []MailSuppression
	p.orgDB(r).Order("created_at DESC").Find(&items)
	return &ListSuppressionsOutput{Body: items}, nil
}

type suppressionDTO struct {
	Address string `json:"address"`
}

type CreateSuppressionInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body suppressionDTO
}

func (i *CreateSuppressionInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateSuppressionOutput struct {
	Body MailSuppression
}

func (p *Plugin) createSuppression(ctx context.Context, input *CreateSuppressionInput) (*CreateSuppressionOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	addr := strings.ToLower(strings.TrimSpace(input.Body.Address))
	if !strings.Contains(addr, "@") {
		return nil, huma.Error400BadRequest("address must be a full email")
	}

	item := MailSuppression{
		OrgID:     orgID,
		Address:   addr,
		Reason:    "manual",
		Source:    "manual",
		Count:     1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := p.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "owner_id"}, {Name: "address"}},
		DoUpdates: clause.Assignments(map[string]any{
			"reason":     "manual",
			"source":     "manual",
			"updated_at": time.Now(),
		}),
	}).Create(&item).Error
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to save suppression")
	}

	if p.audit != nil {
		p.audit(r, "suppression.create", "suppression", item.ID, map[string]any{"address": item.Address, "reason": "manual"})
	}
	return &CreateSuppressionOutput{Body: item}, nil
}

type DeleteSuppressionInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteSuppressionInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteSuppressionOutput struct {
	Body map[string]bool
}

func (p *Plugin) deleteSuppression(ctx context.Context, input *DeleteSuppressionInput) (*DeleteSuppressionOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	var item MailSuppression
	if err := p.orgDB(r).First(&item, input.ID).Error; err != nil {
		return nil, huma.Error404NotFound("suppression item not found")
	}
	if err := p.db.Delete(&item).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to delete suppression")
	}
	if p.audit != nil {
		p.audit(r, "suppression.delete", "suppression", item.ID, map[string]any{"address": item.Address, "reason": item.Reason})
	}
	return &DeleteSuppressionOutput{Body: map[string]bool{"ok": true}}, nil
}

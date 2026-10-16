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
		return nil, huma.Error500InternalServerError("An unexpected error occurred while processing the request context.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
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

const suppressionManual = "manual"

func (p *Plugin) createSuppression(ctx context.Context, input *CreateSuppressionInput) (*CreateSuppressionOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("An unexpected error occurred while processing the request context.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("Administrator role required to manage the suppression list.")
	}
	addr := strings.ToLower(strings.TrimSpace(input.Body.Address))
	if !strings.Contains(addr, "@") {
		return nil, huma.Error400BadRequest("Enter a complete email address including user and domain.")
	}

	now := time.Now()
	item := MailSuppression{
		OrgID:     orgID,
		Address:   addr,
		Reason:    suppressionManual,
		Source:    suppressionManual,
		Count:     1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	err := tdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "owner_id"}, {Name: "address"}},
		DoUpdates: clause.Assignments(map[string]any{
			"reason":     suppressionManual,
			"source":     suppressionManual,
			"updated_at": now,
		}),
	}).Create(&item).Error
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not save the suppression entry. Try again later.")
	}

	if p.audit != nil {
		p.audit(r, "suppression.create", "suppression", item.ID, map[string]any{"address": item.Address, "reason": suppressionManual})
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
		return nil, huma.Error500InternalServerError("An unexpected error occurred while processing the request context.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("Administrator role required to manage the suppression list.")
	}

	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	var item MailSuppression
	if err := tdb.First(&item, input.ID).Error; err != nil {
		return nil, huma.Error404NotFound("Suppression entry not found.")
	}
	if err := tdb.Delete(&item).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not delete the suppression entry. Try again later.")
	}
	if p.audit != nil {
		p.audit(r, "suppression.delete", "suppression", item.ID, map[string]any{"address": item.Address, "reason": item.Reason})
	}
	return &DeleteSuppressionOutput{Body: map[string]bool{"ok": true}}, nil
}

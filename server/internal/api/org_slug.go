package api

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenancy"
	"gorm.io/gorm"
)

var orgSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

type GetOrgSlugInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetOrgSlugInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type OrgSlugView struct {
	Slug string `json:"slug"`
}

type GetOrgSlugOutput struct {
	Body OrgSlugView
}

func (h *Handler) getOrgSlug(ctx context.Context, input *GetOrgSlugInput) (*GetOrgSlugOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, huma.Error403Forbidden("Workspace administrator role required; contact a workspace administrator")
	}

	var org models.Org
	if err := h.db.First(&org, h.auth.OrgID(r)).Error; err != nil {
		return nil, huma.Error404NotFound("Workspace not found; check workspace settings")
	}
	return &GetOrgSlugOutput{Body: OrgSlugView{Slug: org.Slug}}, nil
}

type UpdateOrgSlugInputBody struct {
	Slug string `json:"slug" doc:"lowercase letters, digits and inner hyphens"`
}

type UpdateOrgSlugInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body UpdateOrgSlugInputBody
}

func (i *UpdateOrgSlugInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type UpdateOrgSlugOutput struct {
	Body OrgSlugView
}

func (h *Handler) updateOrgSlug(ctx context.Context, input *UpdateOrgSlugInput) (*UpdateOrgSlugOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleOwner); err != nil {
		return nil, huma.Error403Forbidden("Workspace owner role required to change the workspace slug")
	}

	slug := strings.ToLower(strings.TrimSpace(input.Body.Slug))
	if !orgSlugPattern.MatchString(slug) {
		return nil, huma.Error400BadRequest("Workspace slug must be 3-64 characters of lowercase letters, digits, and hyphens")
	}

	oid := h.auth.OrgID(r)
	var org models.Org
	if err := h.db.First(&org, oid).Error; err != nil {
		return nil, huma.Error404NotFound("Workspace not found; check workspace settings")
	}
	if slug == org.Slug {
		return &UpdateOrgSlugOutput{Body: OrgSlugView{Slug: org.Slug}}, nil
	}

	status, err := models.CheckOrgSlugAvailable(h.db, slug, oid)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not verify slug availability; retry the request")
	}
	if status == models.SlugReserved {
		return nil, huma.Error409Conflict("Workspace slug is reserved; choose a different slug")
	}
	if status == models.SlugTaken {
		return nil, huma.Error409Conflict("Workspace slug is already in use; choose a different slug")
	}

	old := org.Slug
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&org).Update("slug", slug).Error; err != nil {
			return err
		}
		history := models.OrgSlugHistory{
			Slug:      old,
			OrgID:     oid,
			RetiredAt: time.Now(),
		}
		if err := tx.Save(&history).Error; err != nil {
			return err
		}
		if err := tx.Where("slug = ?", slug).Delete(&models.OrgSlugHistory{}).Error; err != nil {
			return err
		}
		if err := tenancy.Retire(tx, oid, old); err != nil {
			return err
		}
		if _, _, err := tenancy.Provision(tx, oid, slug); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, tenancy.ErrNameTaken) {
			return nil, huma.Error409Conflict("Workspace slug is already in use; choose a different slug")
		}
		return nil, huma.Error409Conflict("Workspace slug is already in use; choose a different slug")
	}
	h.audit(r, "org.slug.update", "org", org.ID, map[string]any{"from": old, "to": slug})

	return &UpdateOrgSlugOutput{Body: OrgSlugView{Slug: slug}}, nil
}

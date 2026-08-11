package api

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/authz"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// Renaming a workspace's slug.
//
// Slugs are allocated at random (models.AllocateOrgSlug), which means nobody
// can read one. That is fine for a webhook path and unhelpful for a URL people
// have to recognize, so an owner can choose their own — with the catch that the
// slug is not a display name. It is the address third parties have registered:
//
//	/api/webhook/{orgSlug}/billing/{provider}  — held by Stripe / Polar
//	/api/sso/{orgSlug}/callback                — held by the org's IdP
//
// Changing it breaks both until they are updated by hand, which is why this is
// its own owner-gated endpoint rather than a field on the workspace-rename form
// an admin can submit.

// orgSlugPattern is what a hand-picked slug may look like: lowercase
// alphanumerics and inner hyphens. It is stricter than the generated alphabet
// on purpose — a slug travels in URLs and in provider dashboards, so anything
// that needs escaping or renders ambiguously does not belong in one.
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

// getOrgSlug returns the active workspace's slug and whether it is a leftover
// email-derived one. GET /api/org/slug
func (h *Handler) getOrgSlug(ctx context.Context, input *GetOrgSlugInput) (*GetOrgSlugOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, huma.Error403Forbidden("forbidden")
	}

	var org models.Org
	if err := h.db.First(&org, h.auth.OrgID(r)).Error; err != nil {
		return nil, huma.Error404NotFound("workspace not found")
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

// updateOrgSlug changes the active workspace's slug. Owner only.
// PUT /api/org/slug
func (h *Handler) updateOrgSlug(ctx context.Context, input *UpdateOrgSlugInput) (*UpdateOrgSlugOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	// Owner, not admin: this invalidates addresses held by Stripe and by the
	// org's identity provider, and only the person answerable for those
	// accounts should be able to do it.
	if err := h.requireRole(r, authz.RoleOwner); err != nil {
		return nil, huma.Error403Forbidden("forbidden: only the workspace owner can change the address")
	}

	slug := strings.ToLower(strings.TrimSpace(input.Body.Slug))
	if !orgSlugPattern.MatchString(slug) {
		return nil, huma.Error400BadRequest("address must be 3-64 characters of lowercase letters, digits and inner hyphens")
	}

	oid := h.auth.OrgID(r)
	var org models.Org
	if err := h.db.First(&org, oid).Error; err != nil {
		return nil, huma.Error404NotFound("workspace not found")
	}
	if slug == org.Slug {
		return &UpdateOrgSlugOutput{Body: OrgSlugView{Slug: org.Slug}}, nil
	}

	status, err := models.CheckOrgSlugAvailable(h.db, slug, oid)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to check address availability")
	}
	if status == models.SlugReserved {
		return nil, huma.Error409Conflict("that address is reserved")
	}
	if status == models.SlugTaken {
		// Same answer as "reserved": whether some other workspace holds this
		// address is not something an outsider gets to probe.
		return nil, huma.Error409Conflict("that address is taken")
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
		return nil
	})
	if err != nil {
		// The unique index is the real arbiter; a racing writer lands here.
		return nil, huma.Error409Conflict("that address is taken")
	}
	h.audit(r, "org.slug.update", "org", org.ID, map[string]any{"from": old, "to": slug})

	return &UpdateOrgSlugOutput{Body: OrgSlugView{Slug: slug}}, nil
}

package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenancy"
	"gorm.io/gorm"
)

// callerOrgRole returns the role the current user holds in their active org,
// or "" if they are not a member. Member-management handlers gate on this so a
// plain member can't escalate themselves or evict others.
func (h *Handler) callerOrgRole(r *http.Request) string {
	uid := h.auth.UserID(r)
	oid := h.auth.OrgID(r)
	if uid == 0 || oid == 0 {
		return ""
	}
	var role string
	if err := h.db.Model(&models.OrgMember{}).
		Where("org_id = ? AND user_id = ?", oid, uid).
		Pluck("role", &role).Error; err != nil {
		return ""
	}
	return role
}

// OrgRole is the public wrapper around callerOrgRole, exposed to plugins via
// plugin.Context.OrgRole so a plugin can gate workspace administration on
// owner/admin without importing internal/models.
func (h *Handler) OrgRole(r *http.Request) string {
	// Effective, not raw: plugins gate workspace administration on this, so
	// handing back the holder's membership would let every plugin's gate ignore
	// the token's cap.
	return string(h.effectiveRole(r))
}

type SwitchOrgInputBody struct {
	OrgID uint `json:"orgId"`
}

type SwitchOrgInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body SwitchOrgInputBody
}

func (i *SwitchOrgInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type SwitchOrgOutputBody struct {
	OK bool `json:"ok"`
}

type SwitchOrgOutput struct {
	Body SwitchOrgOutputBody
}

// switchOrg re-issues the session cookie with the new active organization ID.
// POST /api/auth/switch-org  {"orgId": 2}
func (h *Handler) switchOrg(ctx context.Context, input *SwitchOrgInput) (*SwitchOrgOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, w := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	// A bearer token must never leave here holding a session cookie. This is the
	// only endpoint that both accepts a token AND mints a session (every other
	// SetSessionFromRequest call site sits behind a fresh credential: password,
	// OAuth callback, verified external identity, or registration), and minting
	// one here laundered a capped token into an uncapped credential:
	//
	//   - effectiveRole only applies the token's role ceiling while
	//     TokenIDFromContext != 0 (see helpers.go). A session carries no token
	//     ID, so a session minted by a member-capped token came back with the
	//     holder's full owner authority.
	//   - the minted session also outlives the token: revoking or deleting the
	//     token does not touch the sessions table.
	//
	// CSRF is no obstacle either — a cookie-less bearer request is waved through
	// the guard by design. So the refusal has to live here.
	if auth.TokenIDFromContext(r.Context()) != 0 {
		return nil, huma.Error403Forbidden("API tokens cannot switch the active workspace; scope the token to the workspace instead")
	}

	uid := h.auth.UserID(r)
	oldOrg := h.orgID(r)
	// Verify the user belongs to the target organization.
	var mem models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", input.Body.OrgID, uid).First(&mem).Error; err != nil {
		return nil, huma.Error403Forbidden("Account is not a member of this workspace; select an authorized workspace")
	}

	h.auth.SetSessionFromRequest(r, w, uid, input.Body.OrgID)
	// Write the audit row to the workspace being switched INTO, recording the
	// switch explicitly in meta (h.audit would attribute it to the old org).
	h.auditAs(r, input.Body.OrgID, uid, "user.switch_org", "org", input.Body.OrgID,
		map[string]any{"from": oldOrg, "to": input.Body.OrgID})
	out := &SwitchOrgOutput{}
	out.Body.OK = true
	return out, nil
}

type OrgItem struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Role string `json:"role"`
}

type ListOrgsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListOrgsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListOrgsOutput struct {
	Body []OrgItem
}

// listOrgs returns all organizations the current user belongs to.
// GET /api/orgs
func (h *Handler) listOrgs(ctx context.Context, input *ListOrgsInput) (*ListOrgsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	uid := h.auth.UserID(r)
	var items []OrgItem
	err := h.db.Model(&models.OrgMember{}).
		Select("orgs.id, orgs.name, orgs.slug, org_members.role").
		Joins("JOIN orgs ON orgs.id = org_members.org_id").
		Where("org_members.user_id = ?", uid).
		Scan(&items).Error
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not load workspaces; retry the request")
	}

	return &ListOrgsOutput{Body: items}, nil
}

type CreateOrgInputBody struct {
	Name string `json:"name"`
}

type CreateOrgInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body CreateOrgInputBody
}

func (i *CreateOrgInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateOrgOutput struct {
	Body models.Org
}

// createOrg creates a new organization and links the current user as the owner.
// POST /api/orgs  {"name": "New Organization"}
func (h *Handler) createOrg(ctx context.Context, input *CreateOrgInput) (*CreateOrgOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	uid := h.auth.UserID(r)
	name := strings.TrimSpace(input.Body.Name)
	if name == "" {
		return nil, huma.Error400BadRequest("Workspace name is required; provide a workspace name")
	}
	org := models.Org{Name: name}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		slug, err := models.AllocateOrgSlug(tx)
		if err != nil {
			return err
		}
		org.Slug = slug
		if err := tx.Create(&org).Error; err != nil {
			return err
		}
		mem := models.OrgMember{OrgID: org.ID, UserID: uid, Role: "owner"}
		if err := tx.Create(&mem).Error; err != nil {
			return err
		}
		// A configured base domain provisions the org's <slug>.<base> address in
		// the same transaction; an unclaimable address fails the whole create
		// rather than producing a workspace nobody can reach at its address.
		_, _, err = tenancy.Provision(tx, org.ID, org.Slug)
		return err
	})
	if err != nil {
		if errors.Is(err, tenancy.ErrNameTaken) {
			return nil, huma.NewError(http.StatusConflict, "Workspace address could not be claimed; retry the request")
		}
		return nil, huma.Error500InternalServerError("Could not create workspace; retry the request")
	}

	return &CreateOrgOutput{Body: org}, nil
}

type UpdateOrgInputBody struct {
	Name string `json:"name"`
}

type UpdateOrgInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body UpdateOrgInputBody
}

func (i *UpdateOrgInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateOrgOutput struct {
	Body models.Org
}

// updateOrg updates the current organization name.
// PUT /api/org
func (h *Handler) updateOrg(ctx context.Context, input *UpdateOrgInput) (*UpdateOrgOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	oid := h.auth.OrgID(r)

	// Verify permissions: only owner/admin can rename organization/workspace
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, huma.Error403Forbidden("Workspace owner or administrator role required to rename workspace")
	}

	name := strings.TrimSpace(input.Body.Name)
	if name == "" {
		return nil, huma.Error400BadRequest("Workspace name is required; provide a workspace name")
	}

	var org models.Org
	if err := h.db.First(&org, oid).Error; err != nil {
		return nil, huma.Error404NotFound("Workspace not found; verify the workspace identifier and try again")
	}

	org.Name = name
	if err := h.db.Save(&org).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not update workspace name; retry the request")
	}

	return &UpdateOrgOutput{Body: org}, nil
}

type MemberItem struct {
	UserID   uint       `json:"userId"`
	Email    string     `json:"email"`
	Role     string     `json:"role"`
	JoinedAt *time.Time `json:"joinedAt,omitempty"`
	Pending  bool       `json:"pending"`
}

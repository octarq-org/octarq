package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/authz"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/tenancy"
	"github.com/octarq-org/octarq/plugin"
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
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
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
		return nil, huma.Error403Forbidden("api tokens cannot switch the active workspace; scope the token to the workspace instead")
	}

	uid := h.auth.UserID(r)
	oldOrg := h.orgID(r)
	// Verify the user belongs to the target organization.
	var mem models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", input.Body.OrgID, uid).First(&mem).Error; err != nil {
		return nil, huma.Error403Forbidden("not a member of this organization")
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
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	var items []OrgItem
	err := h.db.Model(&models.OrgMember{}).
		Select("orgs.id, orgs.name, orgs.slug, org_members.role").
		Joins("JOIN orgs ON orgs.id = org_members.org_id").
		Where("org_members.user_id = ?", uid).
		Scan(&items).Error
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to query organizations")
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
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	name := strings.TrimSpace(input.Body.Name)
	if name == "" {
		return nil, huma.Error400BadRequest("name is required")
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
			return nil, huma.NewError(http.StatusConflict, "the workspace address could not be claimed — please try again")
		}
		return nil, huma.Error500InternalServerError("failed to create organization")
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
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	oid := h.auth.OrgID(r)

	// Verify permissions: only owner/admin can rename organization/workspace
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, huma.Error403Forbidden("forbidden: only owner/admin can rename workspace")
	}

	name := strings.TrimSpace(input.Body.Name)
	if name == "" {
		return nil, huma.Error400BadRequest("name is required")
	}

	var org models.Org
	if err := h.db.First(&org, oid).Error; err != nil {
		return nil, huma.Error404NotFound("workspace not found")
	}

	org.Name = name
	if err := h.db.Save(&org).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to update workspace name")
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

type MenuItem struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
	Order    int    `json:"order,omitempty"`
	// Mirrors plugin.MenuItem.RequiredRole.
	RequiredRole string `json:"requiredRole,omitempty"`
}

type ListMenusInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListMenusInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListMenusOutput struct {
	Body []MenuItem
}

// listMenus aggregates core menus and all plugin-registered menus.
// GET /api/menus
func (h *Handler) listMenus(ctx context.Context, input *ListMenusInput) (*ListMenusOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Core default navigation items — ONLY the pages the core itself serves
	// (plus the Infrastructure asset placeholders it owns). The backend is the
	// source of truth for which paths are "real", so the frontend drops any
	// composed menu whose path no backend half announces (see the sidebar merge
	// in web/src/App.tsx). Feature plugins (links, mail, dns, …) announce their
	// own entries via MenuProvider below, so a disabled plugin's path is never
	// offered.
	menus := []MenuItem{
		{ID: "overview", Label: "Overview", Path: "/overview", Icon: "layout-dashboard", Category: "Workspace"},

		{ID: "audit", Label: "Audit Log", Path: "/audit", Icon: "scroll-text", Category: "System"},
		{ID: "abuse", Label: "Abuse Reports", Path: "/abuse", Icon: "shield-alert", Category: "Security"},
	}

	// Query from plugin providers if they satisfy MenuProvider — but only for
	// features the caller's workspace has active (core plumbing is always on;
	// everything else follows its per-workspace toggle).
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	for _, p := range h.plugins {
		if !h.pluginActive(orgID, p) {
			continue
		}
		if mp, ok := p.(plugin.MenuProvider); ok {
			for _, m := range mp.Menus() {
				menus = append(menus, MenuItem{
					ID:           m.ID,
					Label:        m.Label,
					Path:         m.Path,
					Icon:         m.Icon,
					Category:     m.Category,
					Order:        m.Order,
					RequiredRole: m.RequiredRole,
				})
			}
		}
	}

	return &ListMenusOutput{Body: menus}, nil
}

type Action struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
	Order    int    `json:"order,omitempty"`
	// Mirrors plugin.Action.RequiredRole.
	RequiredRole string `json:"requiredRole,omitempty"`
}

type ListActionsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListActionsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListActionsOutput struct {
	Body []Action
}

// listActions aggregates plugin-registered actions for global create menu.
// GET /api/actions
func (h *Handler) listActions(ctx context.Context, input *ListActionsInput) (*ListActionsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	actions := make([]Action, 0)

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	for _, p := range h.plugins {
		if !h.pluginActive(orgID, p) {
			continue
		}
		if ap, ok := p.(plugin.ActionProvider); ok {
			for _, a := range ap.Actions() {
				actions = append(actions, Action{
					ID:           a.ID,
					Label:        a.Label,
					Path:         a.Path,
					Icon:         a.Icon,
					Category:     a.Category,
					Order:        a.Order,
					RequiredRole: a.RequiredRole,
				})
			}
		}
	}

	return &ListActionsOutput{Body: actions}, nil
}

type GetUserSettingsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetUserSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetUserSettingsOutput struct {
	Body map[string]string
}

// getUserSettings returns the current user's preference key/value pairs.
// GET /api/user/settings
//
// The only key in use today is onboarding_dismissed, read by the Overview page.
func (h *Handler) getUserSettings(ctx context.Context, input *GetUserSettingsInput) (*GetUserSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	var settings []models.UserSetting
	if err := h.db.Where("user_id = ?", uid).Find(&settings).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to query user settings")
	}

	out := make(map[string]string)
	for _, s := range settings {
		out[s.Key] = s.Value
	}
	return &GetUserSettingsOutput{Body: out}, nil
}

type UpdateUserSettingsInputBody struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type UpdateUserSettingsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body UpdateUserSettingsInputBody
}

func (i *UpdateUserSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateUserSettingsOutputBody struct {
	OK bool `json:"ok"`
}

type UpdateUserSettingsOutput struct {
	Body UpdateUserSettingsOutputBody
}

// updateUserSettings sets or updates a single user preference.
// PUT /api/user/settings  {"key": "onboarding_dismissed", "value": "true"}
func (h *Handler) updateUserSettings(ctx context.Context, input *UpdateUserSettingsInput) (*UpdateUserSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	key := strings.TrimSpace(input.Body.Key)
	if key == "" {
		return nil, huma.Error400BadRequest("key is required")
	}

	s := models.UserSetting{
		UserID:    uid,
		Key:       key,
		Value:     input.Body.Value,
		UpdatedAt: time.Now(),
	}
	if err := h.db.Save(&s).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save user setting")
	}
	out := &UpdateUserSettingsOutput{}
	out.Body.OK = true
	return out, nil
}

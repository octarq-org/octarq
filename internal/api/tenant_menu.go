package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
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

type SwitchOrgInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		OrgID uint `json:"orgId"`
	}
}

func (i *SwitchOrgInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type SwitchOrgOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
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

	uid := h.auth.UserID(r)
	// Verify the user belongs to the target organization.
	var mem models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", input.Body.OrgID, uid).First(&mem).Error; err != nil {
		return nil, huma.Error403Forbidden("not a member of this organization")
	}

	h.auth.SetSessionFromRequest(r, w, uid, input.Body.OrgID)
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

type CreateOrgInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Name string `json:"name"`
	}
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
	slug := safeSlug(name)
	if slug == "" {
		slug = "org-" + time.Now().Format("20060102150405")
	}

	// Ensure slug uniqueness.
	orig := slug
	for i := 1; ; i++ {
		var count int64
		h.db.Model(&models.Org{}).Where("slug = ?", slug).Count(&count)
		if count == 0 {
			break
		}
		slug = fmt.Sprintf("%s-%d", orig, i)
	}

	org := models.Org{Name: name, Slug: slug}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&org).Error; err != nil {
			return err
		}
		mem := models.OrgMember{OrgID: org.ID, UserID: uid, Role: "owner"}
		return tx.Create(&mem).Error
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create organization")
	}

	return &CreateOrgOutput{Body: org}, nil
}

type UpdateOrgInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Name string `json:"name"`
	}
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

	uid := h.auth.UserID(r)
	oid := h.auth.OrgID(r)

	// Verify permissions: only owner/admin can rename organization/workspace
	var role string
	err := h.db.Model(&models.OrgMember{}).
		Where("org_id = ? AND user_id = ?", oid, uid).
		Pluck("role", &role).Error
	if err != nil || (role != "owner" && role != "admin") {
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

type ListOrgMembersInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListOrgMembersInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListOrgMembersOutput struct {
	Body []MemberItem
}

// listOrgMembers lists all members and their roles for the current active organization.
// GET /api/org/members
func (h *Handler) listOrgMembers(ctx context.Context, input *ListOrgMembersInput) (*ListOrgMembersOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	orgID := h.orgID(r)
	items := []MemberItem{}
	type queryResult struct {
		UserID      uint
		Email       string
		Role        string
		InviteToken string
		CreatedAt   time.Time
	}
	var rows []queryResult
	err := h.db.Table("org_members").
		Select("users.id as user_id, users.email, org_members.role, users.invite_token, users.created_at").
		Joins("JOIN users ON users.id = org_members.user_id").
		Where("org_members.org_id = ?", orgID).
		Scan(&rows).Error
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to query members")
	}
	for _, row := range rows {
		// Pending means an unredeemed invite. An empty password hash alone is
		// NOT pending: the bootstrap instance admin authenticates against the
		// configured env password and never stores a hash.
		isPending := row.InviteToken != ""
		t := row.CreatedAt
		items = append(items, MemberItem{
			UserID:   row.UserID,
			Email:    row.Email,
			Role:     row.Role,
			JoinedAt: &t,
			Pending:  isPending,
		})
	}
	return &ListOrgMembersOutput{Body: items}, nil
}

type AddOrgMemberInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
}

func (i *AddOrgMemberInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AddOrgMemberOutput struct {
	Body map[string]any
}

// addOrgMember adds or invites a user to the active organization.
// POST /api/org/members  {"email": "user@example.com", "role": "member"}
func (h *Handler) addOrgMember(ctx context.Context, input *AddOrgMemberInput) (*AddOrgMemberOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	orgID := h.orgID(r)
	callerRole := h.callerOrgRole(r)
	if callerRole != "owner" && callerRole != "admin" {
		return nil, huma.Error403Forbidden("forbidden: only owner/admin can manage members")
	}

	email := strings.TrimSpace(input.Body.Email)
	role := strings.TrimSpace(input.Body.Role)
	if email == "" {
		return nil, huma.Error400BadRequest("email is required")
	}
	if role != "owner" && role != "admin" && role != "member" {
		role = "member"
	}
	// Only an owner may grant or revoke the owner role — an admin can't mint
	// owners (or promote itself) and thereby take over the workspace.
	if role == "owner" && callerRole != "owner" {
		return nil, huma.Error403Forbidden("forbidden: only an owner can grant the owner role")
	}

	// Find or create the target User.
	var user models.User
	var isNew bool
	if err := h.db.Where("email = ?", email).First(&user).Error; err != nil {
		isNew = true
		tokenBytes := make([]byte, 24)
		if _, err := rand.Read(tokenBytes); err != nil {
			return nil, huma.Error500InternalServerError("failed to generate invite token")
		}
		token := hex.EncodeToString(tokenBytes)
		expiresAt := time.Now().Add(24 * time.Hour)

		user = models.User{
			Email:           email,
			PasswordHash:    "",
			InviteToken:     token,
			InviteExpiresAt: &expiresAt,
		}
		if err := h.db.Create(&user).Error; err != nil {
			return nil, huma.Error500InternalServerError("failed to create user")
		}
	}

	// Check if already a member.
	var existing models.OrgMember
	memErr := h.db.Where("org_id = ? AND user_id = ?", orgID, user.ID).First(&existing).Error
	if memErr == nil {
		// Re-grading an existing owner (demote, or re-affirm) is an owner-only act,
		// so an admin can't strip the owner's role out from under them.
		if existing.Role == "owner" && callerRole != "owner" {
			return nil, huma.Error403Forbidden("forbidden: only an owner can change an owner's role")
		}
		if err := h.db.Model(&models.OrgMember{}).
			Where("org_id = ? AND user_id = ?", orgID, user.ID).
			Update("role", role).Error; err != nil {
			return nil, huma.Error500InternalServerError("failed to update member role")
		}
	} else {
		mem := models.OrgMember{OrgID: orgID, UserID: user.ID, Role: role}
		if err := h.db.Create(&mem).Error; err != nil {
			return nil, huma.Error500InternalServerError("failed to add member")
		}
	}

	h.audit(r, "member.add", "user", user.ID, map[string]any{"email": user.Email, "role": role})
	eventbus.Publish(orgID, "member.invite", map[string]any{"userId": user.ID, "email": user.Email, "role": role, "pending": user.InviteToken != ""})

	if isNew {
		// Best-effort: email the invite link via the org's SMTP sender. A missing
		// sender (or a send error) must not fail the invite — the link is still
		// returned so the operator can deliver it out-of-band.
		acceptURL := "/admin/invite/accept?token=" + user.InviteToken
		if h.cfg.BaseURL != "" {
			acceptURL = h.cfg.BaseURL + acceptURL
		}
		h.sendInviteEmail(orgID, email, acceptURL)
		return &AddOrgMemberOutput{
			Body: map[string]any{
				"ok":          true,
				"inviteToken": user.InviteToken,
				"inviteUrl":   "/admin/invite/accept?token=" + user.InviteToken,
			},
		}, nil
	}
	return &AddOrgMemberOutput{
		Body: map[string]any{"ok": true},
	}, nil
}

// sendInviteEmail best-effort delivers the invite accept link to the invited
// address using the org's first configured SMTP sender. It never returns an
// error: a missing sender or a send failure is logged and swallowed so the
// invite itself still succeeds.
func (h *Handler) sendInviteEmail(orgID uint, to, acceptURL string) {
	if sendMail, ok := h.LookupService("mail.send"); ok {
		if fn, ok := sendMail.(func(orgID uint, to, subject, htmlBody, textBody string) error); ok {
			text := fmt.Sprintf("You've been invited to join a workspace on octarq.\n\n"+
				"Accept your invite and set a password here:\n%s\n\n"+
				"This link expires in 24 hours.", acceptURL)
			if err := fn(orgID, to, "You've been invited to octarq", "", text); err != nil {
				log.Printf("invite email to %s failed: %v", to, err)
			}
			return
		}
	}
	log.Printf("invite email skipped for %s: mail plugin not mounted", to)
}

type RemoveOrgMemberInput struct {
	Ctx    huma.Context `hidden:"true"`
	UserID uint         `path:"userId"`
}

func (i *RemoveOrgMemberInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type RemoveOrgMemberOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// removeOrgMember removes a user from the active organization.
// DELETE /api/org/members/{userId}
func (h *Handler) removeOrgMember(ctx context.Context, input *RemoveOrgMemberInput) (*RemoveOrgMemberOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	orgID := h.orgID(r)
	callerRole := h.callerOrgRole(r)
	if callerRole != "owner" && callerRole != "admin" {
		return nil, huma.Error403Forbidden("forbidden: only owner/admin can manage members")
	}

	callerUID := h.auth.UserID(r)
	if input.UserID == callerUID {
		return nil, huma.Error400BadRequest("cannot remove yourself from the workspace")
	}

	var target models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, input.UserID).First(&target).Error; err != nil {
		return nil, huma.Error404NotFound("not a member of this organization")
	}
	// Only an owner may remove an owner.
	if target.Role == "owner" && callerRole != "owner" {
		return nil, huma.Error403Forbidden("forbidden: only an owner can remove an owner")
	}
	// Never strand the workspace ownerless — refuse to remove the last owner.
	if target.Role == "owner" {
		var owners int64
		h.db.Model(&models.OrgMember{}).Where("org_id = ? AND role = ?", orgID, "owner").Count(&owners)
		if owners <= 1 {
			return nil, huma.Error400BadRequest("cannot remove the last owner of the workspace")
		}
	}

	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, input.UserID).
		Delete(&models.OrgMember{}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to remove member")
	}
	h.audit(r, "member.remove", "user", input.UserID, map[string]any{"role": target.Role})
	eventbus.Publish(orgID, "member.remove", map[string]any{"userId": input.UserID, "role": target.Role})
	out := &RemoveOrgMemberOutput{}
	out.Body.OK = true
	return out, nil
}

type MenuItem struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
	Order    int    `json:"order,omitempty"`
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
		{ID: "overview", Label: "Overview", Path: "/overview", Icon: "📊", Category: "Operations"},

		{ID: "audit", Label: "Audit Log", Path: "/audit", Icon: "📝", Category: "Compliance"},
		{ID: "abuse", Label: "Abuse", Path: "/abuse", Icon: "🛡️", Category: "Compliance"},

		{ID: "certs", Label: "Certificates", Path: "/assets/certificates", Icon: "🔒", Category: "Network", Order: 20},
		{ID: "databases", Label: "Databases", Path: "/assets/databases", Icon: "🗄️", Category: "Storage & Databases"},
		{ID: "storage", Label: "Object Storage", Path: "/assets/storage", Icon: "💾", Category: "Storage & Databases"},
	}

	// Query from plugin providers if they satisfy MenuProvider — but only for
	// features the caller's workspace has active (core plumbing is always on;
	// everything else follows its per-workspace toggle).
	orgID := h.orgID(r)
	for _, p := range h.plugins {
		if !h.pluginActive(orgID, p) {
			continue
		}
		if mp, ok := p.(plugin.MenuProvider); ok {
			for _, m := range mp.Menus() {
				menus = append(menus, MenuItem{
					ID:       m.ID,
					Label:    m.Label,
					Path:     m.Path,
					Icon:     m.Icon,
					Category: m.Category,
					Order:    m.Order,
				})
			}
		}
	}

	return &ListMenusOutput{Body: menus}, nil
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

// getUserSettings returns all settings (such as custom menu groupings) for the current user.
// GET /api/user/settings
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

type UpdateUserSettingsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
}

func (i *UpdateUserSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateUserSettingsOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// updateUserSettings sets or updates a specific user preference key-value pair.
// PUT /api/user/settings  {"key": "menu_layout", "value": "{...}"}
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

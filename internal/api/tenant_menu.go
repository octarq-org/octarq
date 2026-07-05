package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jungley8/led/internal/mail"
	"github.com/Jungley8/led/internal/models"
	"github.com/Jungley8/led/plugin"
	"gorm.io/gorm"
)

// userIdParam extracts the userId parameter from the URL path.
func userIdParam(r *http.Request) (uint, bool) {
	s := r.PathValue("userId")
	if s == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

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

// switchOrg re-issues the session cookie with the new active organization ID.
// POST /api/auth/switch-org  {"orgId": 2}
func (h *Handler) switchOrg(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		OrgID uint `json:"orgId"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	// Verify the user belongs to the target organization.
	var mem models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", body.OrgID, uid).First(&mem).Error; err != nil {
		writeErr(w, http.StatusForbidden, "not a member of this organization")
		return
	}

	h.auth.SetSessionFromRequest(r, w, uid, body.OrgID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// listOrgs returns all organizations the current user belongs to.
// GET /api/orgs
func (h *Handler) listOrgs(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type OrgItem struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
		Role string `json:"role"`
	}

	var items []OrgItem
	err := h.db.Model(&models.OrgMember{}).
		Select("orgs.id, orgs.name, orgs.slug, org_members.role").
		Joins("JOIN orgs ON orgs.id = org_members.org_id").
		Where("org_members.user_id = ?", uid).
		Scan(&items).Error
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to query organizations")
		return
	}

	writeJSON(w, http.StatusOK, items)
}

// createOrg creates a new organization and links the current user as the owner.
// POST /api/orgs  {"name": "New Organization"}
func (h *Handler) createOrg(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	slug := safeSlug(name)
	if slug == "" {
		slug = "org-" + strconv.FormatInt(time.Now().Unix(), 10)
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
		writeErr(w, http.StatusInternalServerError, "failed to create organization")
		return
	}

	writeJSON(w, http.StatusCreated, org)
}

// updateOrg updates the current organization name.
// PUT /api/org
func (h *Handler) updateOrg(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	oid := h.auth.OrgID(r)
	if uid == 0 || oid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Verify permissions: only owner/admin can rename organization/workspace
	var role string
	err := h.db.Model(&models.OrgMember{}).
		Where("org_id = ? AND user_id = ?", oid, uid).
		Pluck("role", &role).Error
	if err != nil || (role != "owner" && role != "admin") {
		writeErr(w, http.StatusForbidden, "forbidden: only owner/admin can rename workspace")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	var org models.Org
	if err := h.db.First(&org, oid).Error; err != nil {
		writeErr(w, http.StatusNotFound, "workspace not found")
		return
	}

	org.Name = name
	if err := h.db.Save(&org).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update workspace name")
		return
	}

	writeJSON(w, http.StatusOK, org)
}

// listOrgMembers lists all members and their roles for the current active organization.
// GET /api/org/members
func (h *Handler) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	orgID := h.orgID(r)
	type MemberItem struct {
		UserID uint   `json:"userId"`
		Email  string `json:"email"`
		Role   string `json:"role"`
	}
	items := []MemberItem{}
	err := h.db.Model(&models.OrgMember{}).
		Select("users.id as user_id, users.email, org_members.role").
		Joins("JOIN users ON users.id = org_members.user_id").
		Where("org_members.org_id = ?", orgID).
		Scan(&items).Error
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to query members")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// addOrgMember adds or invites a user to the active organization.
// POST /api/org/members  {"email": "user@example.com", "role": "member"}
func (h *Handler) addOrgMember(w http.ResponseWriter, r *http.Request) {
	orgID := h.orgID(r)
	callerRole := h.callerOrgRole(r)
	if callerRole != "owner" && callerRole != "admin" {
		writeErr(w, http.StatusForbidden, "forbidden: only owner/admin can manage members")
		return
	}
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	email := strings.TrimSpace(body.Email)
	role := strings.TrimSpace(body.Role)
	if email == "" {
		writeErr(w, http.StatusBadRequest, "email is required")
		return
	}
	if role != "owner" && role != "admin" && role != "member" {
		role = "member"
	}
	// Only an owner may grant or revoke the owner role — an admin can't mint
	// owners (or promote itself) and thereby take over the workspace.
	if role == "owner" && callerRole != "owner" {
		writeErr(w, http.StatusForbidden, "forbidden: only an owner can grant the owner role")
		return
	}

	// Find or create the target User.
	var user models.User
	var isNew bool
	if err := h.db.Where("email = ?", email).First(&user).Error; err != nil {
		isNew = true
		tokenBytes := make([]byte, 24)
		if _, err := rand.Read(tokenBytes); err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to generate invite token")
			return
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
			writeErr(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	}

	// Check if already a member.
	var existing models.OrgMember
	memErr := h.db.Where("org_id = ? AND user_id = ?", orgID, user.ID).First(&existing).Error
	if memErr == nil {
		// Re-grading an existing owner (demote, or re-affirm) is an owner-only act,
		// so an admin can't strip the owner's role out from under them.
		if existing.Role == "owner" && callerRole != "owner" {
			writeErr(w, http.StatusForbidden, "forbidden: only an owner can change an owner's role")
			return
		}
		if err := h.db.Model(&models.OrgMember{}).
			Where("org_id = ? AND user_id = ?", orgID, user.ID).
			Update("role", role).Error; err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to update member role")
			return
		}
	} else {
		mem := models.OrgMember{OrgID: orgID, UserID: user.ID, Role: role}
		if err := h.db.Create(&mem).Error; err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to add member")
			return
		}
	}

	h.audit(r, "member.add", "user", user.ID, map[string]any{"email": user.Email, "role": role})

	if isNew {
		// Best-effort: email the invite link via the org's SMTP sender. A missing
		// sender (or a send error) must not fail the invite — the link is still
		// returned so the operator can deliver it out-of-band.
		acceptURL := "/admin/invite/accept?token=" + user.InviteToken
		if h.cfg.BaseURL != "" {
			acceptURL = h.cfg.BaseURL + acceptURL
		}
		h.sendInviteEmail(orgID, email, acceptURL)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"inviteToken": user.InviteToken,
			"inviteUrl":   "/admin/invite/accept?token=" + user.InviteToken,
		})
	} else {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// sendInviteEmail best-effort delivers the invite accept link to the invited
// address using the org's first configured SMTP sender. It never returns an
// error: a missing sender or a send failure is logged and swallowed so the
// invite itself still succeeds.
func (h *Handler) sendInviteEmail(orgID uint, to, acceptURL string) {
	var s models.SMTPSender
	if err := h.db.Where("owner_id = ?", orgID).Order("id").First(&s).Error; err != nil {
		log.Printf("invite email skipped for %s: no SMTP sender for org %d", to, orgID)
		return
	}
	pass, err := h.cipher.Decrypt(s.Pass)
	if err != nil {
		log.Printf("invite email skipped for %s: decrypt SMTP pass: %v", to, err)
		return
	}
	sender := mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)
	msg := mail.Message{
		From:    s.FromEmail,
		To:      []string{to},
		Subject: "You've been invited to led",
		Text: fmt.Sprintf("You've been invited to join a workspace on led.\n\n"+
			"Accept your invite and set a password here:\n%s\n\n"+
			"This link expires in 24 hours.", acceptURL),
	}
	if err := sender.Send(msg); err != nil {
		log.Printf("invite email to %s failed: %v", to, err)
	}
}

// removeOrgMember removes a user from the active organization.
// DELETE /api/org/members/{userId}
func (h *Handler) removeOrgMember(w http.ResponseWriter, r *http.Request) {
	orgID := h.orgID(r)
	callerRole := h.callerOrgRole(r)
	if callerRole != "owner" && callerRole != "admin" {
		writeErr(w, http.StatusForbidden, "forbidden: only owner/admin can manage members")
		return
	}
	targetUID, ok := userIdParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad userId")
		return
	}

	var target models.OrgMember
	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, targetUID).First(&target).Error; err != nil {
		writeErr(w, http.StatusNotFound, "not a member of this organization")
		return
	}
	// Only an owner may remove an owner.
	if target.Role == "owner" && callerRole != "owner" {
		writeErr(w, http.StatusForbidden, "forbidden: only an owner can remove an owner")
		return
	}
	// Never strand the workspace ownerless — refuse to remove the last owner.
	if target.Role == "owner" {
		var owners int64
		h.db.Model(&models.OrgMember{}).Where("org_id = ? AND role = ?", orgID, "owner").Count(&owners)
		if owners <= 1 {
			writeErr(w, http.StatusBadRequest, "cannot remove the last owner of the workspace")
			return
		}
	}

	if err := h.db.Where("org_id = ? AND user_id = ?", orgID, targetUID).
		Delete(&models.OrgMember{}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	h.audit(r, "member.remove", "user", targetUID, map[string]any{"role": target.Role})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// listMenus aggregates core menus and all plugin-registered menus.
// GET /api/menus
func (h *Handler) listMenus(w http.ResponseWriter, r *http.Request) {
	type MenuItem struct {
		ID       string `json:"id"`
		Label    string `json:"label"`
		Path     string `json:"path"`
		Icon     string `json:"icon"`
		Category string `json:"category"`
	}

	// Core default navigation items
	menus := []MenuItem{
		{ID: "overview", Label: "Overview", Path: "/overview", Icon: "📊", Category: "Operations"},
		{ID: "links", Label: "Links", Path: "/links", Icon: "🔗", Category: "Operations"},
		{ID: "domains", Label: "Domains", Path: "/domains", Icon: "🌐", Category: "Assets"},
		{ID: "mail", Label: "Mail", Path: "/mail", Icon: "✉️", Category: "Operations"},

		{ID: "audit", Label: "Audit Log", Path: "/audit", Icon: "📝", Category: "Compliance"},
		{ID: "abuse", Label: "Abuse", Path: "/abuse", Icon: "🛡️", Category: "Compliance"},
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
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, menus)
}

// getUserSettings returns all settings (such as custom menu groupings) for the current user.
// GET /api/user/settings
func (h *Handler) getUserSettings(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var settings []models.UserSetting
	if err := h.db.Where("user_id = ?", uid).Find(&settings).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to query user settings")
		return
	}

	out := make(map[string]string)
	for _, s := range settings {
		out[s.Key] = s.Value
	}
	writeJSON(w, http.StatusOK, out)
}

// updateUserSettings sets or updates a specific user preference key-value pair.
// PUT /api/user/settings  {"key": "menu_layout", "value": "{...}"}
func (h *Handler) updateUserSettings(w http.ResponseWriter, r *http.Request) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Key = strings.TrimSpace(body.Key)
	if body.Key == "" {
		writeErr(w, http.StatusBadRequest, "key is required")
		return
	}

	s := models.UserSetting{
		UserID:    uid,
		Key:       body.Key,
		Value:     body.Value,
		UpdatedAt: time.Now(),
	}
	if err := h.db.Save(&s).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save user setting")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

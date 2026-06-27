package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	h.auth.SetSession(w, uid, body.OrgID)
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

// listOrgMembers lists all members and their roles for the current active organization.
// GET /api/org/members
func (h *Handler) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	orgID := h.orgID(r)
	type MemberItem struct {
		UserID uint   `json:"userId"`
		Email  string `json:"email"`
		Role   string `json:"role"`
	}
	var items []MemberItem
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

	// Find or create the target User.
	var user models.User
	if err := h.db.Where("email = ?", email).First(&user).Error; err != nil {
		user = models.User{Email: email, PasswordHash: ""}
		if err := h.db.Create(&user).Error; err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	}

	// Check if already a member.
	var count int64
	h.db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", orgID, user.ID).Count(&count)
	if count > 0 {
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

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// removeOrgMember removes a user from the active organization.
// DELETE /api/org/members/{userId}
func (h *Handler) removeOrgMember(w http.ResponseWriter, r *http.Request) {
	orgID := h.orgID(r)
	targetUID, ok := userIdParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad userId")
		return
	}

	err := h.db.Where("org_id = ? AND user_id = ?", orgID, targetUID).
		Delete(&models.OrgMember{}).Error
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
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
		{ID: "overview", Label: "Overview", Path: "/overview", Icon: "📊", Category: "Traffic"},
		{ID: "links", Label: "Links", Path: "/links", Icon: "🔗", Category: "Traffic"},
		{ID: "domains", Label: "Domains", Path: "/domains", Icon: "🌐", Category: "Traffic"},
		{ID: "mail", Label: "Mail", Path: "/mail", Icon: "✉️", Category: "Traffic"},

		{ID: "audit", Label: "Audit Log", Path: "/audit", Icon: "📝", Category: "Infrastructure"},
		{ID: "abuse", Label: "Abuse", Path: "/abuse", Icon: "🛡️", Category: "Infrastructure"},
		{ID: "settings", Label: "Settings", Path: "/settings", Icon: "⚙️", Category: "Infrastructure"},
	}

	// Query from plugin providers if they satisfy MenuProvider
	for _, p := range h.plugins {
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

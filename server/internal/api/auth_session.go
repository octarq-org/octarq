package api

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/models"
)

type LogoutAllInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *LogoutAllInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LogoutAllOutputBody struct {
	OK bool `json:"ok"`
}

type LogoutAllOutput struct {
	Body LogoutAllOutputBody
}

func (h *Handler) logoutAll(ctx context.Context, input *LogoutAllInput) (*LogoutAllOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, w := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)
	oid := h.orgID(r)
	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Find(&sessions)
	ctxCtx := r.Context()
	for _, s := range sessions {
		if err := h.auth.Cache().Delete(ctxCtx, "session:"+s.Token); err != nil {
			slog.Warn("failed to delete session cache", "token", s.Token, "err", err)
		}
	}
	h.db.Where("user_id = ?", uid).Delete(&models.Session{})
	h.auth.Clear(r, w)
	h.auditAs(r, oid, uid, "user.logout_all", "session", 0, map[string]any{"sessionsRevoked": len(sessions)})
	out := &LogoutAllOutput{}
	out.Body.OK = true
	return out, nil
}

type ListSessionsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListSessionsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type SessionRow struct {
	ID         uint      `json:"id"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"userAgent"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	CreatedAt  time.Time `json:"createdAt"`
	IsCurrent  bool      `json:"isCurrent"`
	Location   string    `json:"location,omitempty"`
}

type ListSessionsOutput struct {
	Body []SessionRow
}

func (h *Handler) listSessions(ctx context.Context, input *ListSessionsInput) (*ListSessionsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)
	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Order("last_seen_at DESC").Limit(20).Find(&sessions)

	currID := h.auth.SessionID(r)
	out := make([]SessionRow, len(sessions))
	for i, s := range sessions {
		ipClean := strings.Trim(s.IP, "[]")
		maskedIP := maskIPServer(ipClean)

		var location string
		if ipClean == "::1" || ipClean == "127.0.0.1" {
			location = "Localhost"
		} else if h.geo != nil {
			country, _, city := h.geo.Locate(ipClean)
			location = locationFromGeo(country, city)
		}
		out[i] = SessionRow{
			ID:         s.ID,
			IP:         maskedIP,
			UserAgent:  s.UserAgent,
			LastSeenAt: s.LastSeenAt,
			CreatedAt:  s.CreatedAt,
			IsCurrent:  s.ID == currID,
			Location:   location,
		}
	}
	return &ListSessionsOutput{Body: out}, nil
}

func locationFromGeo(country, city string) string {
	parts := make([]string, 0, 2)
	if city != "" {
		parts = append(parts, city)
	}
	if country != "" {
		parts = append(parts, country)
	}
	return strings.Join(parts, ", ")
}

func maskIPServer(ip string) string {
	if ip == "::1" || ip == "127.0.0.1" {
		return ip
	}
	if parts := strings.Split(ip, "."); len(parts) == 4 {
		return parts[0] + "." + parts[1] + "." + parts[2] + ".*"
	}
	if idx := strings.LastIndex(ip, ":"); idx >= 0 {
		return ip[:idx] + ":*"
	}
	return ip
}

type RevokeSessionInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *RevokeSessionInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type RevokeSessionOutputBody struct {
	OK   bool `json:"ok"`
	Self bool `json:"self"`
}

type RevokeSessionOutput struct {
	Body RevokeSessionOutputBody
}

func (h *Handler) revokeSession(ctx context.Context, input *RevokeSessionInput) (*RevokeSessionOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, w := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)
	oid := h.orgID(r)
	var sess models.Session
	if err := h.db.Where("id = ? AND user_id = ?", input.ID, uid).First(&sess).Error; err != nil {
		return nil, huma.Error404NotFound("Session not found; check the session identifier and try again")
	}
	h.db.Delete(&sess)
	if err := h.auth.Cache().Delete(r.Context(), "session:"+sess.Token); err != nil {
		slog.Warn("failed to delete session cache", "token", sess.Token, "err", err)
	}
	h.auditAs(r, oid, uid, "user.session_revoke", "session", sess.ID, nil)

	isSelf := h.auth.SessionID(r) == sess.ID
	if isSelf {
		h.auth.Clear(r, w)
	}
	out := &RevokeSessionOutput{}
	out.Body.OK = true
	out.Body.Self = isSelf
	return out, nil
}

type LogoutInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *LogoutInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LogoutOutputBody struct {
	OK bool `json:"ok"`
}

type LogoutOutput struct {
	Body LogoutOutputBody
}

func (h *Handler) logout(ctx context.Context, input *LogoutInput) (*LogoutOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, w := humago.Unwrap(input.Ctx)
	uid := h.auth.UserID(r)
	oid := h.orgID(r)
	sid := h.auth.SessionID(r)
	h.auth.Clear(r, w)
	h.auditAs(r, oid, uid, "user.logout", "session", sid, nil)
	out := &LogoutOutput{}
	out.Body.OK = true
	return out, nil
}

type MeInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *MeInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type MeOutputBody struct {
	Email           string `json:"email"`
	Username        string `json:"username,omitempty"`
	OrgID           uint   `json:"orgId"`
	Role            string `json:"role"`
	EmailVerified   bool   `json:"emailVerified"`
	IsInstanceAdmin bool   `json:"isInstanceAdmin"`
}

type MeOutput struct {
	Body MeOutputBody
}

func (h *Handler) me(ctx context.Context, input *MeInput) (*MeOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("User account not found; sign in again or check your account status")
	}
	out := &MeOutput{}
	out.Body.Email = user.Email
	out.Body.Username = user.Email
	out.Body.OrgID = h.orgID(r)
	out.Body.Role = string(h.effectiveRole(r))
	out.Body.EmailVerified = user.EmailVerified
	out.Body.IsInstanceAdmin = user.IsInstanceAdmin
	return out, nil
}

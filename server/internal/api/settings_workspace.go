package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenancy"
)

// currentOrg loads the caller's org, generating its inbound-webhook token on
// first read so the operator always has a token to copy into the worker URL.
func (h *Handler) currentOrg(r *http.Request) (models.Org, error) {
	if _, err := h.requireOrg(r); err != nil {
		return models.Org{}, err
	}
	var org models.Org
	h.db.First(&org, h.orgID(r))
	if org.ID != 0 && org.InboundToken == "" {
		org.InboundToken = uuid.NewString()
		h.db.Model(&org).Update("inbound_token", org.InboundToken)
	}
	return org, nil
}

// tenantSubdomain returns the org's automatic tenant address under the
// configured base domain, or "" when no base is configured. It is the read
// side of the provisioning on org creation, so the dashboard can show the
// address without importing the dns plugin.
func (h *Handler) tenantSubdomain(slug string) string {
	name, _ := tenancy.Subdomain(h.db, slug)
	return name
}

type GetSettingsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetSettingsOutput struct {
	Body map[string]any
}

func (h *Handler) getSettings(ctx context.Context, input *GetSettingsInput) (*GetSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	org, err := h.currentOrg(r)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"reservedMailboxes": h.GetWorkspaceSetting(orgID, keyReservedMailboxes),
		"orgSlug":           org.Slug,
		// The org's automatic tenant subdomain, when a base domain is
		// configured; empty string = no base, so the org has only its slug.
		"tenantSubdomain": h.tenantSubdomain(org.Slug),
		"catchAll":        h.GetWorkspaceSetting(orgID, keyCatchAll) == "true",
		"autoWrapLinks":   h.GetWorkspaceSetting(orgID, keyAutoWrapLinks) == "true",
		"isInstanceAdmin": h.isInstanceAdmin(r),
	}
	// inboundTokenSet reports whether the org's inbound-email webhook secret is
	// set, without exposing it. Whoever holds the token can forge inbound mail
	// for the workspace, so the plaintext is never part of a settings dump — it
	// is fetched explicitly through the admin-only GET /api/settings/inbound-token
	// endpoint instead. Gate through callerHoldsRole rather than comparing
	// callerOrgRole directly — that is the one function that also understands API
	// tokens. Comparing the membership role alone hid the field from every
	// bearer-authenticated caller, since a token has no membership row.
	if h.callerHoldsRole(r, authz.RoleAdmin) {
		body["inboundTokenSet"] = org.InboundToken != ""
	}
	out := &GetSettingsOutput{
		Body: body,
	}
	return out, nil
}

type GetInboundTokenInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetInboundTokenInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetInboundTokenOutputBody struct {
	InboundToken string `json:"inboundToken"`
}

type GetInboundTokenOutput struct {
	Body GetInboundTokenOutputBody
}

// getInboundToken is the only endpoint that returns the org's inbound-email
// webhook secret in full. getSettings now answers with a boolean (inboundTokenSet),
// so the raw token never rides along in a settings dump; fetching it is an
// explicit, admin-or-owner-only action.
func (h *Handler) getInboundToken(ctx context.Context, input *GetInboundTokenInput) (*GetInboundTokenOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if !h.callerHoldsRole(r, authz.RoleAdmin) {
		return nil, huma.Error403Forbidden("Workspace owner or administrator role required; contact a workspace administrator")
	}
	if _, err := h.requireOrg(r); err != nil {
		return nil, err
	}
	org, err := h.currentOrg(r)
	if err != nil {
		return nil, err
	}
	out := &GetInboundTokenOutput{}
	out.Body.InboundToken = org.InboundToken
	return out, nil
}

type UpdateSettingsInputBody struct {
	ReservedMailboxes *string `json:"reservedMailboxes,omitempty"`
	InboundToken      *string `json:"inboundToken,omitempty"`
	CatchAll          *bool   `json:"catchAll,omitempty"`
	AutoWrapLinks     *bool   `json:"autoWrapLinks,omitempty"`
}

type UpdateSettingsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body UpdateSettingsInputBody
}

func (i *UpdateSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateSettingsOutput struct {
	Body map[string]any
}

func (h *Handler) updateSettings(ctx context.Context, input *UpdateSettingsInput) (*UpdateSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, huma.Error403Forbidden("Workspace owner or administrator role required; contact a workspace administrator")
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}

	if input.Body.ReservedMailboxes != nil {
		h.SetWorkspaceSetting(orgID, keyReservedMailboxes, strings.Join(splitList(*input.Body.ReservedMailboxes), "\n"))
	}
	if input.Body.InboundToken != nil {
		// Per-org: empty string rotates to a fresh UUID; a value sets it explicitly.
		tok := strings.TrimSpace(*input.Body.InboundToken)
		if tok == "" {
			tok = uuid.NewString()
		}
		h.db.Model(&models.Org{}).Where("id = ?", orgID).Update("inbound_token", tok)
	}
	if input.Body.CatchAll != nil {
		val := "false"
		if *input.Body.CatchAll {
			val = "true"
		}
		h.SetWorkspaceSetting(orgID, keyCatchAll, val)
	}

	if input.Body.AutoWrapLinks != nil {
		val := "false"
		if *input.Body.AutoWrapLinks {
			val = "true"
		}
		h.SetWorkspaceSetting(orgID, keyAutoWrapLinks, val)
	}

	meta := make(map[string]any)
	if input.Body.ReservedMailboxes != nil {
		meta["reservedMailboxes"] = *input.Body.ReservedMailboxes
	}
	if input.Body.InboundToken != nil {
		meta["inboundToken"] = "[REDACTED]"
	}
	if input.Body.CatchAll != nil {
		meta["catchAll"] = *input.Body.CatchAll
	}
	if input.Body.AutoWrapLinks != nil {
		meta["autoWrapLinks"] = *input.Body.AutoWrapLinks
	}
	h.audit(r, "settings.update", "settings", 0, meta)

	org, err := h.currentOrg(r)
	if err != nil {
		return nil, err
	}
	out := &UpdateSettingsOutput{
		Body: map[string]any{
			"reservedMailboxes": h.GetWorkspaceSetting(org.ID, keyReservedMailboxes),
			"orgSlug":           org.Slug,
			"tenantSubdomain":   h.tenantSubdomain(org.Slug),
			"inboundTokenSet":   org.InboundToken != "",
			"catchAll":          h.GetWorkspaceSetting(org.ID, keyCatchAll) == "true",
			"autoWrapLinks":     h.GetWorkspaceSetting(org.ID, keyAutoWrapLinks) == "true",
			"isInstanceAdmin":   h.isInstanceAdmin(r),
		},
	}
	return out, nil
}

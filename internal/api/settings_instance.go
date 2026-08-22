package api

import (
	"context"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/origin"
)

type GetInstanceSettingsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetInstanceSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetInstanceSettingsOutput struct {
	Body map[string]any
}

func (h *Handler) getInstanceSettings(ctx context.Context, input *GetInstanceSettingsInput) (*GetInstanceSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("instance admin role required")
	}
	retDays := DefaultRetentionDays
	if v := h.getSetting(keyDataRetentionDays); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			retDays = n
		}
	}
	out := &GetInstanceSettingsOutput{
		Body: map[string]any{
			"reservedSlugs":            h.getSetting(keyReservedSlugs),
			"builtinReserved":          []string{"admin", "api", "assets", "portal"},
			"googleClientId":           h.getSetting(keyGoogleClientID),
			"googleClientSecretSet":    h.getSetting(keyGoogleClientSecret) != "",
			"githubClientId":           h.getSetting(keyGitHubClientID),
			"githubClientSecretSet":    h.getSetting(keyGitHubClientSecret) != "",
			"dataRetentionDays":        retDays,
			"allowRegistration":        h.registrationEnabled(),
			"requireEmailVerification": h.requireEmailVerification(),
			"appName":                  h.getSetting(keyAppName), // raw value; empty = default
			"baseDomain":               models.BaseDomain(h.db),  // effective value incl. the OCTARQ_BASE_DOMAIN bootstrap fallback
			"sharedHosts":              h.sharedHostsSetting(),
			"metricsTokenSet":          h.getSetting(keyMetricsToken) != "",
			"ratelimitAuthRpm":         h.settingInt(keyRatelimitAuthRPM, defaultAuthRPM),
			"ratelimitApiRpm":          h.settingInt(keyRatelimitAPIRPM, defaultAPIRPM),
			"ratelimitRedirectRpm":     h.settingInt(keyRatelimitRedirRPM, defaultRedirectRPM),
			"publicCorsOrigins":        h.getSetting(keyPublicCORSOrigins),
			"systemSenderId":           h.systemSenderID(),
		},
	}
	return out, nil
}

type UpdateInstanceSettingsInputBody struct {
	ReservedSlugs            *string `json:"reservedSlugs,omitempty"`
	GoogleClientID           *string `json:"googleClientId,omitempty"`
	GoogleClientSecret       *string `json:"googleClientSecret,omitempty"`
	GitHubClientID           *string `json:"githubClientId,omitempty"`
	GitHubClientSecret       *string `json:"githubClientSecret,omitempty"`
	DataRetentionDays        *int    `json:"dataRetentionDays,omitempty"`
	AllowRegistration        *bool   `json:"allowRegistration,omitempty"`
	RequireEmailVerification *bool   `json:"requireEmailVerification,omitempty"`
	AppName                  *string `json:"appName,omitempty"`
	BaseDomain               *string `json:"baseDomain,omitempty"`
	SharedHosts              *string `json:"sharedHosts,omitempty"`
	MetricsToken             *string `json:"metricsToken,omitempty"`
	RatelimitAuthRpm         *int    `json:"ratelimitAuthRpm,omitempty"`
	RatelimitApiRpm          *int    `json:"ratelimitApiRpm,omitempty"`
	RatelimitRedirectRpm     *int    `json:"ratelimitRedirectRpm,omitempty"`
	PublicCORSOrigins        *string `json:"publicCorsOrigins,omitempty"`
	SystemSenderID           *uint   `json:"systemSenderId,omitempty"`
}

type UpdateInstanceSettingsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body UpdateInstanceSettingsInputBody
}

func (i *UpdateInstanceSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateInstanceSettingsOutput struct {
	Body map[string]any
}

func (h *Handler) updateInstanceSettings(ctx context.Context, input *UpdateInstanceSettingsInput) (*UpdateInstanceSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("instance admin role required")
	}

	if input.Body.ReservedSlugs != nil {
		h.setSetting(keyReservedSlugs, strings.Join(splitList(*input.Body.ReservedSlugs), "\n"))
	}
	if input.Body.GoogleClientID != nil {
		h.setSetting(keyGoogleClientID, strings.TrimSpace(*input.Body.GoogleClientID))
	}
	if input.Body.GoogleClientSecret != nil {
		if *input.Body.GoogleClientSecret == "" {
			h.setSetting(keyGoogleClientSecret, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*input.Body.GoogleClientSecret)))
			if err != nil {
				return nil, huma.Error500InternalServerError("encrypt token")
			}
			h.setSetting(keyGoogleClientSecret, enc)
		}
	}
	if input.Body.GitHubClientID != nil {
		h.setSetting(keyGitHubClientID, strings.TrimSpace(*input.Body.GitHubClientID))
	}
	if input.Body.GitHubClientSecret != nil {
		if *input.Body.GitHubClientSecret == "" {
			h.setSetting(keyGitHubClientSecret, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*input.Body.GitHubClientSecret)))
			if err != nil {
				return nil, huma.Error500InternalServerError("encrypt token")
			}
			h.setSetting(keyGitHubClientSecret, enc)
		}
	}
	if input.Body.DataRetentionDays != nil {
		h.setSetting(keyDataRetentionDays, strconv.Itoa(*input.Body.DataRetentionDays))
	}
	if input.Body.AllowRegistration != nil {
		val := "false"
		if *input.Body.AllowRegistration {
			val = "true"
		}
		h.setSetting(keyAllowRegistration, val)
	}
	if input.Body.RequireEmailVerification != nil {
		val := "false"
		if *input.Body.RequireEmailVerification {
			val = "true"
		}
		h.setSetting(keyRequireEmailVerification, val)
	}
	if input.Body.AppName != nil {
		h.setSetting(keyAppName, strings.TrimSpace(*input.Body.AppName))
	}
	if input.Body.BaseDomain != nil {
		h.setSetting(keyBaseDomain, strings.TrimSpace(strings.ToLower(*input.Body.BaseDomain)))
		// Origin caches the reserved zone (and every ownership answer computed
		// against it) for minutes; a base-domain change must be visible now, so
		// drop the whole namespace.
		origin.ClearBaseDomainCache(h.db)
	}
	if input.Body.SharedHosts != nil {
		h.setSetting(keySharedHosts, strings.Join(splitList(*input.Body.SharedHosts), "\n"))
		origin.ClearSharedHostsCache(h.db)
	}
	if input.Body.MetricsToken != nil {
		if *input.Body.MetricsToken == "" {
			h.setSetting(keyMetricsToken, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*input.Body.MetricsToken)))
			if err != nil {
				return nil, huma.Error500InternalServerError("encrypt token")
			}
			h.setSetting(keyMetricsToken, enc)
		}
	}
	if input.Body.RatelimitAuthRpm != nil {
		h.setSetting(keyRatelimitAuthRPM, strconv.Itoa(*input.Body.RatelimitAuthRpm))
	}
	if input.Body.RatelimitApiRpm != nil {
		h.setSetting(keyRatelimitAPIRPM, strconv.Itoa(*input.Body.RatelimitApiRpm))
	}
	if input.Body.PublicCORSOrigins != nil {
		h.setSetting(keyPublicCORSOrigins, strings.Join(splitList(*input.Body.PublicCORSOrigins), "\n"))
	}
	if input.Body.SystemSenderID != nil {
		if *input.Body.SystemSenderID == 0 {
			h.setSetting(keySystemSenderID, "")
		} else {
			h.setSetting(keySystemSenderID, strconv.FormatUint(uint64(*input.Body.SystemSenderID), 10))
		}
	}
	meta := make(map[string]any)
	if input.Body.ReservedSlugs != nil {
		meta["reservedSlugs"] = *input.Body.ReservedSlugs
	}
	if input.Body.GoogleClientID != nil {
		meta["googleClientId"] = *input.Body.GoogleClientID
	}
	if input.Body.GoogleClientSecret != nil {
		meta["googleClientSecret"] = "[REDACTED]"
	}
	if input.Body.GitHubClientID != nil {
		meta["githubClientId"] = *input.Body.GitHubClientID
	}
	if input.Body.GitHubClientSecret != nil {
		meta["githubClientSecret"] = "[REDACTED]"
	}
	if input.Body.DataRetentionDays != nil {
		meta["dataRetentionDays"] = *input.Body.DataRetentionDays
	}
	if input.Body.AllowRegistration != nil {
		meta["allowRegistration"] = *input.Body.AllowRegistration
	}
	if input.Body.AppName != nil {
		meta["appName"] = *input.Body.AppName
	}
	if input.Body.BaseDomain != nil {
		meta["baseDomain"] = *input.Body.BaseDomain
	}
	if input.Body.SharedHosts != nil {
		meta["sharedHosts"] = *input.Body.SharedHosts
	}
	if input.Body.MetricsToken != nil {
		meta["metricsToken"] = "[REDACTED]"
	}
	if input.Body.RatelimitAuthRpm != nil {
		meta["ratelimitAuthRpm"] = *input.Body.RatelimitAuthRpm
	}
	if input.Body.RatelimitApiRpm != nil {
		meta["ratelimitApiRpm"] = *input.Body.RatelimitApiRpm
	}
	if input.Body.RatelimitRedirectRpm != nil {
		meta["ratelimitRedirectRpm"] = *input.Body.RatelimitRedirectRpm
	}
	if input.Body.PublicCORSOrigins != nil {
		meta["publicCorsOrigins"] = *input.Body.PublicCORSOrigins
	}
	if input.Body.SystemSenderID != nil {
		meta["systemSenderId"] = *input.Body.SystemSenderID
	}
	h.audit(r, "instance_settings.update", "settings", 0, meta)

	retDays := DefaultRetentionDays
	if v := h.getSetting(keyDataRetentionDays); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			retDays = n
		}
	}
	out := &UpdateInstanceSettingsOutput{
		Body: map[string]any{
			"reservedSlugs":         h.getSetting(keyReservedSlugs),
			"builtinReserved":       []string{"admin", "api", "assets", "portal"},
			"googleClientId":        h.getSetting(keyGoogleClientID),
			"googleClientSecretSet": h.getSetting(keyGoogleClientSecret) != "",
			"githubClientId":        h.getSetting(keyGitHubClientID),
			"githubClientSecretSet": h.getSetting(keyGitHubClientSecret) != "",
			"dataRetentionDays":     retDays,
			"allowRegistration":     h.registrationEnabled(),
			"appName":               h.getSetting(keyAppName),
			"baseDomain":            models.BaseDomain(h.db),
			"sharedHosts":           h.sharedHostsSetting(),
			"metricsTokenSet":       h.getSetting(keyMetricsToken) != "",
			"ratelimitAuthRpm":      h.settingInt(keyRatelimitAuthRPM, defaultAuthRPM),
			"ratelimitApiRpm":       h.settingInt(keyRatelimitAPIRPM, defaultAPIRPM),
			"ratelimitRedirectRpm":  h.settingInt(keyRatelimitRedirRPM, defaultRedirectRPM),
			"publicCorsOrigins":     h.getSetting(keyPublicCORSOrigins),
			"systemSenderId":        h.systemSenderID(),
		},
	}
	return out, nil
}

func (h *Handler) sharedHostsSetting() string {
	if v := h.getSetting(keySharedHosts); v != "" {
		return v
	}
	return strings.Join(models.SharedHosts(h.db), "\n")
}

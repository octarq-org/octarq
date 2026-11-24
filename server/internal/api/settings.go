package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm/clause"
)

const (
	keyGoogleClientID           = "oauth.google.client_id"
	vaultKeyGoogleClientSecret  = "instance.oauth.google"
	keyGitHubClientID           = "oauth.github.client_id"
	vaultKeyGitHubClientSecret  = "instance.oauth.github"
	keyDataRetentionDays        = "data_retention_days"
	keyAllowRegistration        = "allow_registration"
	keyAppName                  = "app_name"
	keyBrandLogo                = "brand_logo"
	keyBrandColor               = "brand_color"
	keyBrandColor2              = "brand_color_2"
	vaultKeyMetricsToken        = "instance.metrics_token"
	keyRatelimitAuthRPM         = "ratelimit_auth_rpm"
	keyRatelimitAPIRPM          = "ratelimit_api_rpm"
	keyRequireEmailVerification = "require_email_verification"
	keyBaseDomain               = models.BaseDomainSetting
	keySharedHosts              = models.SharedHostsSetting
	keyPublicCORSOrigins        = "public_cors_origins"
)

const (
	defaultAuthRPM = 60
	defaultAPIRPM  = 600
)

func (h *Handler) brandSetting(orgID uint, key string) string {
	if orgID != 0 {
		if v := strings.TrimSpace(h.GetWorkspaceSetting(orgID, key)); v != "" {
			return v
		}
	}
	return strings.TrimSpace(h.getSetting(key))
}

func (h *Handler) AppName() string { return h.AppNameFor(0) }

func (h *Handler) AppNameFor(orgID uint) string {
	if v := h.brandSetting(orgID, keyAppName); v != "" {
		return v
	}
	return config.DefaultAppName
}

func (h *Handler) Brand() (logo, color, color2 string) { return h.BrandFor(0) }

func (h *Handler) BrandFor(orgID uint) (logo, color, color2 string) {
	return h.brandSetting(orgID, keyBrandLogo),
		h.brandSetting(orgID, keyBrandColor),
		h.brandSetting(orgID, keyBrandColor2)
}

func (h *Handler) MetricsToken() string {
	if tok := h.resolveInstanceSecret(context.Background(), vaultKeyMetricsToken, "token", "value"); tok != "" {
		return tok
	}
	if h.cfg != nil && h.cfg.MetricsToken != "" {
		return strings.TrimSpace(h.cfg.MetricsToken)
	}
	return strings.TrimSpace(os.Getenv("OCTARQ_METRICS_TOKEN"))
}

func (h *Handler) resolveInstanceSecret(ctx context.Context, keyName string, fieldKeys ...string) string {
	vs := h.vault()
	if vs == nil {
		return ""
	}
	sec, err := vs.ResolveInstanceSecret(ctx, keyName)
	if err != nil || sec == nil {
		return ""
	}
	for _, fk := range fieldKeys {
		if s, ok := sec.Payload[fk].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func (h *Handler) hasInstanceSecret(ctx context.Context, keyName string, fieldKeys ...string) bool {
	return h.resolveInstanceSecret(ctx, keyName, fieldKeys...) != ""
}

func (h *Handler) setInstanceSecret(ctx context.Context, keyName, displayName, fieldKey, value string, actorUserID uint) error {
	vs := h.vault()
	if vs == nil {
		return errors.New("vault service unavailable")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		if _, err := vs.ResolveInstanceSecret(ctx, keyName); errors.Is(err, plugin.ErrSecretNotFound) {
			return nil
		}
		_, err := vs.PutInstanceSecret(ctx, plugin.PutSecretRequest{
			ItemType:    "instance_secret",
			KeyName:     keyName,
			Name:        displayName,
			Payload:     map[string]any{fieldKey: "", "configured": false},
			ChangeNote:  "cleared via instance settings",
			ActorUserID: actorUserID,
		})
		return err
	}
	_, err := vs.PutInstanceSecret(ctx, plugin.PutSecretRequest{
		ItemType:    "instance_secret",
		KeyName:     keyName,
		Name:        displayName,
		Payload:     map[string]any{fieldKey: value, "configured": true},
		ChangeNote:  "updated via instance settings",
		ActorUserID: actorUserID,
	})
	return err
}

func (h *Handler) RateLimits() (authRPM, apiRPM int) {
	defAuth := defaultAuthRPM
	if v := os.Getenv("OCTARQ_RATELIMIT_AUTH_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			defAuth = n
		}
	}
	defAPI := defaultAPIRPM
	if v := os.Getenv("OCTARQ_RATELIMIT_API_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			defAPI = n
		}
	}
	return h.settingInt(keyRatelimitAuthRPM, defAuth),
		h.settingInt(keyRatelimitAPIRPM, defAPI)
}

func (h *Handler) settingInt(key string, def int) int {
	v := strings.TrimSpace(h.getSetting(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func (h *Handler) CORSOrigins() []string {
	if v := h.getSetting(keyPublicCORSOrigins); v != "" {
		return splitList(v)
	}
	return splitList(h.cfg.PublicCORSOrigins)
}

func (h *Handler) registrationEnabled() bool {
	return h.getSetting(keyAllowRegistration) != "false"
}

func (h *Handler) requireEmailVerification() bool {
	return h.getSetting(keyRequireEmailVerification) != "false"
}

func (h *Handler) RequireEmailVerification() bool {
	return h.requireEmailVerification()
}

const DefaultRetentionDays = 90

func (h *Handler) getSetting(key string) string {
	var s models.Setting
	if h.db.First(&s, "key = ?", key).Error == nil {
		return s.Value
	}
	return ""
}

func (h *Handler) setSetting(key, value string) error {
	return h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&models.Setting{Key: key, Value: value}).Error
}

func (h *Handler) GetWorkspaceSetting(orgID uint, key string) string {
	if orgID == 0 {
		return ""
	}
	var s models.WorkspaceSetting
	if h.db.First(&s, "org_id = ? AND key = ?", orgID, key).Error == nil {
		return s.Value
	}
	return ""
}

func (h *Handler) SetWorkspaceSetting(orgID uint, key, value string) error {
	if orgID == 0 {
		return tenantsql.ErrZeroOrgID
	}
	return h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&models.WorkspaceSetting{OrgID: orgID, Key: key, Value: value}).Error
}

func splitList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	seen := map[string]bool{}
	var out []string
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

func (h *Handler) isInstanceAdmin(r *http.Request) bool {
	uid := h.auth.UserID(r)
	if uid == 0 {
		return false
	}
	var isAdmin bool
	if err := h.db.Model(&models.User{}).
		Where("id = ?", uid).
		Pluck("is_instance_admin", &isAdmin).Error; err != nil {
		return false
	}
	return isAdmin
}

func (h *Handler) IsInstanceAdmin(r *http.Request) bool {
	return h.isInstanceAdmin(r)
}

func (h *Handler) GetGlobalSetting(key string) string {
	return h.getSetting(key)
}

func (h *Handler) SetGlobalSetting(key, value string) error {
	return h.setSetting(key, value)
}

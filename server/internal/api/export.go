package api

import (
	"archive/zip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func (h *Handler) tenantDB(orgID uint) *plugin.TenantDB {
	tdb, err := plugin.NewTenantDB(h.db, orgID)
	if err != nil {
		return nil
	}
	return tdb
}

func (h *Handler) GenerateExportSignedToken(orgID uint, expire time.Duration) (string, error) {
	if orgID == 0 {
		return "", errors.New("invalid org ID")
	}
	if expire <= 0 {
		expire = 15 * time.Minute
	}
	if h.storage != nil {
		return h.storage.GenerateDownloadToken(fmt.Sprintf("export:%d", orgID), expire), nil
	}
	secret := crypto.DeriveKey([]byte(h.cfg.SecretKey), crypto.PurposeDownloadToken)
	expTime := time.Now().Add(expire).Unix()
	fileID := fmt.Sprintf("export:%d", orgID)
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "download:%s:%d", fileID, expTime)
	sig := hex.EncodeToString(mac.Sum(nil))
	raw := fmt.Sprintf("%s.%d.%s", fileID, expTime, sig)
	return base64.RawURLEncoding.EncodeToString([]byte(raw)), nil
}

func (h *Handler) GenerateExportSignedURL(orgID uint) (string, error) {
	tok, err := h.GenerateExportSignedToken(orgID, 15*time.Minute)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/api/account/export?token=%s", url.QueryEscape(tok)), nil
}

type ExportAccountInput struct {
	Ctx   huma.Context `hidden:"true"`
	Token string       `query:"token" doc:"Optional 15-minute temporary signed download token"`
}

func (i *ExportAccountInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (h *Handler) exportAccount(ctx context.Context, input *ExportAccountInput) (*huma.StreamResponse, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)

	var org uint
	var actorID uint

	if strings.TrimSpace(input.Token) != "" {
		valid := false
		if h.storage != nil {
			fileID, _, ok := h.storage.ParseDownloadToken(input.Token)
			if ok && strings.HasPrefix(fileID, "export:") {
				if parsed, err := strconv.ParseUint(strings.TrimPrefix(fileID, "export:"), 10, 64); err == nil && parsed > 0 {
					org = uint(parsed)
					valid = true
				}
			}
		} else if h.cfg != nil && h.cfg.SecretKey != "" {
			if decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(input.Token)); err == nil {
				parts := strings.Split(string(decoded), ".")
				if len(parts) == 3 && strings.HasPrefix(parts[0], "export:") {
					exp, _ := strconv.ParseInt(parts[1], 10, 64)
					if time.Now().Unix() <= exp {
						mac := hmac.New(sha256.New, crypto.DeriveKey([]byte(h.cfg.SecretKey), crypto.PurposeDownloadToken))
						fmt.Fprintf(mac, "download:%s:%d", parts[0], exp)
						expectedSig := hex.EncodeToString(mac.Sum(nil))
						if hmac.Equal([]byte(expectedSig), []byte(parts[2])) {
							if parsed, err := strconv.ParseUint(strings.TrimPrefix(parts[0], "export:"), 10, 64); err == nil && parsed > 0 {
								org = uint(parsed)
								valid = true
							}
						}
					}
				}
			}
		}
		if !valid {
			return nil, huma.Error401Unauthorized("Download token is invalid or expired; request a new download token")
		}
	} else {
		r2, ok := h.auth.AuthenticateRequest(r)
		if !ok {
			return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
		}
		if err := h.requireRole(r2, authz.RoleAdmin); err != nil {
			return nil, err
		}
		var err error
		org, err = h.requireOrg(r2)
		if err != nil {
			return nil, err
		}
		actorID = h.auth.UserID(r2)
	}

	auditEntry := models.AuditLog{
		OrgID:      org,
		ActorID:    actorID,
		Action:     "account.data_export",
		TargetType: "account",
		TargetID:   org,
		IP:         reporterIP(r),
		CreatedAt:  time.Now().UTC(),
	}
	if err := h.db.Create(&auditEntry).Error; err != nil {
		slog.Error("failed to record data_export audit entry", "orgId", org, "err", err)
		return nil, huma.Error500InternalServerError("Failed to record export security audit event")
	}

	tdb := h.tenantDB(org)
	if tdb == nil {
		return nil, huma.Error500InternalServerError("Failed to initialize tenant database handle")
	}

	var orgRow models.Org
	h.db.Where("id = ?", org).First(&orgRow)

	pluginData := make(map[string]map[string]any)
	pluginNames := make([]string, 0, len(h.plugins))
	for _, p := range h.plugins {
		fn, ok := plugin.LookupServiceAs[plugin.ExportFunc](h.LookupService, plugin.ExportServiceName(p.Name()))
		if !ok {
			continue
		}
		data := fn(org)
		if len(data) == 0 {
			continue
		}
		name := p.Name()
		pluginData[name] = data
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)

	ninetyDaysAgo := time.Now().UTC().Add(-90 * 24 * time.Hour)
	var auditLogs []models.AuditLog
	tdb.Scoped(&models.AuditLog{}).Where("created_at >= ?", ninetyDaysAgo).Order("created_at DESC").Find(&auditLogs)

	files := make([]string, 0, len(pluginNames)+2)
	files = append(files, "metadata.json")
	for _, name := range pluginNames {
		files = append(files, fmt.Sprintf("plugins/%s.json", name))
	}
	files = append(files, "audit_logs.json")

	metadata := map[string]any{
		"exportedAt":    time.Now().UTC().Format(time.RFC3339),
		"orgId":         org,
		"workspaceName": orgRow.Name,
		"version":       "1.0.0",
		"files":         files,
		"counts": map[string]int64{
			"plugins":   int64(len(pluginNames)),
			"auditLogs": int64(len(auditLogs)),
		},
		"note": "Secret material (token hashes, encrypted credentials, external SMTP passwords, webhook secrets) is intentionally excluded or redacted.",
	}

	filename := fmt.Sprintf("workspace-export-org%d-%s.zip", org, time.Now().UTC().Format("20060102-150405"))

	return &huma.StreamResponse{
		Body: func(hctx huma.Context) {
			hctx.SetHeader("Content-Type", "application/zip")
			hctx.SetHeader("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
			hctx.SetHeader("Cache-Control", "no-store, no-cache, must-revalidate")

			w := hctx.BodyWriter()
			zw := zip.NewWriter(w)

			addJSON := func(name string, v any) error {
				zf, err := zw.Create(name)
				if err != nil {
					return err
				}
				enc := json.NewEncoder(zf)
				enc.SetIndent("", "  ")
				return enc.Encode(v)
			}

			if err := addJSON("metadata.json", metadata); err != nil {
				slog.Warn("export: failed writing metadata.json", "err", err)
			}

			for _, name := range pluginNames {
				if err := addJSON(fmt.Sprintf("plugins/%s.json", name), pluginData[name]); err != nil {
					slog.Warn("export: failed writing plugin data", "plugin", name, "err", err)
				}
			}

			if err := addJSON("audit_logs.json", auditLogs); err != nil {
				slog.Warn("export: failed writing audit_logs.json", "err", err)
			}

			if err := zw.Close(); err != nil {
				slog.Error("export: error closing zip writer", "err", err)
			}
		},
	}, nil
}

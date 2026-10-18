package api

import (
	"archive/zip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

// tenantDB returns a restricted *plugin.TenantDB strictly scoped to orgID.
func (h *Handler) tenantDB(orgID uint) *plugin.TenantDB {
	tdb, err := plugin.NewTenantDB(h.db, orgID)
	if err != nil {
		return nil
	}
	return tdb
}

// GenerateExportSignedToken generates a 15-minute temporary download token for full tenant export.
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
	secret := []byte(h.cfg.SecretKey)
	expTime := time.Now().Add(expire).Unix()
	fileID := fmt.Sprintf("export:%d", orgID)
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "download:%s:%d", fileID, expTime)
	sig := hex.EncodeToString(mac.Sum(nil))
	raw := fmt.Sprintf("%s.%d.%s", fileID, expTime, sig)
	return base64.RawURLEncoding.EncodeToString([]byte(raw)), nil
}

// GenerateExportSignedURL creates a download URL containing a 15-minute HMAC-signed token.
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

var secretTokenRegex = regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9_\-]{6,}|ghp_[a-zA-Z0-9]{10,}|bearer\s+[a-zA-Z0-9_\-\.]{10,})`)

func sanitizeSecret(s string) string {
	return secretTokenRegex.ReplaceAllStringFunc(s, func(m string) string {
		if len(m) >= 3 && strings.EqualFold(m[:3], "sk-") {
			return "sk-****"
		}
		return "[redacted]"
	})
}

type exportLinkRow struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     uint      `gorm:"column:owner_id" json:"-"`
	Host      string    `json:"host"`
	Slug      string    `json:"slug"`
	Target    string    `json:"target"`
	Title     string    `json:"title"`
	Clicks    int64     `json:"clicks"`
	Tags      string    `json:"tags"`
	CreatedAt time.Time `json:"createdAt"`
}

func (exportLinkRow) TableName() string    { return "links" }
func (exportLinkRow) TenantColumn() string { return "owner_id" }

type exportDomainRow struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	OrgID             uint      `gorm:"column:owner_id" json:"-"`
	Name              string    `json:"name"`
	ProviderAccountID uint      `json:"providerAccountId"`
	ZoneID            string    `json:"zoneId"`
	Note              string    `json:"note"`
	ForMail           bool      `json:"forMail"`
	ForLink           bool      `json:"forLink"`
	LinkHosts         string    `json:"linkHosts"`
	MailHosts         string    `json:"mailHosts"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (exportDomainRow) TableName() string    { return "domains" }
func (exportDomainRow) TenantColumn() string { return "owner_id" }

type exportDomainItem struct {
	exportDomainRow
	Records []plugin.DNSRecord `json:"records"`
}

type exportMailboxRow struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     uint      `gorm:"column:owner_id" json:"-"`
	Address   string    `json:"address"`
	Note      string    `json:"note"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (exportMailboxRow) TableName() string    { return "mailboxes" }
func (exportMailboxRow) TenantColumn() string { return "owner_id" }

type mailboxStats struct {
	TotalReceived int64 `json:"totalReceived"`
	Unread        int64 `json:"unread"`
}

type exportMailboxItem struct {
	exportMailboxRow
	Stats mailboxStats `json:"stats"`
}

// exportAccount streams the full archive of a tenant workspace as a ZIP file.
// GET /api/account/export
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
						mac := hmac.New(sha256.New, []byte(h.cfg.SecretKey))
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

	// 1. Fail-closed synchronous audit entry
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

	// 2. Strict tenant database handle
	tdb := h.tenantDB(org)
	if tdb == nil {
		return nil, huma.Error500InternalServerError("Failed to initialize tenant database handle")
	}

	// 3. Query all tenant resources
	var orgRow models.Org
	h.db.Where("id = ?", org).First(&orgRow)

	var linkRows []exportLinkRow
	tdb.Table("links").Order("id ASC").Find(&linkRows)

	var domRows []exportDomainRow
	tdb.Table("domains").Order("id ASC").Find(&domRows)

	var dnsMgr plugin.DNSManager
	if h.lookupService != nil {
		if mgr, ok := plugin.LookupServiceAs[plugin.DNSManager](h.LookupService, plugin.ServiceDNSManager); ok {
			dnsMgr = mgr
		}
	}

	domainItems := make([]exportDomainItem, len(domRows))
	for i, d := range domRows {
		domainItems[i] = exportDomainItem{
			exportDomainRow: d,
			Records:         []plugin.DNSRecord{},
		}
		if dnsMgr != nil {
			if recs, err := dnsMgr.List(ctx, org, d.ID); err == nil && recs != nil {
				sanitized := make([]plugin.DNSRecord, len(recs))
				for j, rec := range recs {
					rec.Content = sanitizeSecret(rec.Content)
					rec.Comment = sanitizeSecret(rec.Comment)
					sanitized[j] = rec
				}
				domainItems[i].Records = sanitized
			}
		}
	}

	var mbRows []exportMailboxRow
	tdb.Table("mailboxes").Order("id ASC").Find(&mbRows)

	mailboxItems := make([]exportMailboxItem, len(mbRows))
	for i, m := range mbRows {
		var total, unread int64
		h.db.Table("emails").Where("mailbox_id = ?", m.ID).Count(&total)
		h.db.Table("emails").Where("mailbox_id = ? AND read = ?", m.ID, false).Count(&unread)
		mailboxItems[i] = exportMailboxItem{
			exportMailboxRow: m,
			Stats: mailboxStats{
				TotalReceived: total,
				Unread:        unread,
			},
		}
	}

	ninetyDaysAgo := time.Now().UTC().Add(-90 * 24 * time.Hour)
	var auditLogs []models.AuditLog
	tdb.Scoped(&models.AuditLog{}).Where("created_at >= ?", ninetyDaysAgo).Order("created_at DESC").Find(&auditLogs)

	metadata := map[string]any{
		"exportedAt":    time.Now().UTC().Format(time.RFC3339),
		"orgId":         org,
		"workspaceName": orgRow.Name,
		"version":       "1.0.0",
		"files": []string{
			"metadata.json",
			"links.csv",
			"domains.json",
			"mailboxes.json",
			"audit_logs.json",
		},
		"counts": map[string]int64{
			"links":     int64(len(linkRows)),
			"domains":   int64(len(domainItems)),
			"mailboxes": int64(len(mailboxItems)),
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

			// 1. metadata.json
			if err := addJSON("metadata.json", metadata); err != nil {
				slog.Warn("export: failed writing metadata.json", "err", err)
			}

			// 2. links.csv
			if zf, err := zw.Create("links.csv"); err == nil {
				cw := csv.NewWriter(zf)
				if err := cw.Write([]string{"ID", "Host", "Slug", "Target", "Title", "Clicks", "Tags", "CreatedAt"}); err != nil {
					slog.Warn("export: failed writing links.csv header", "err", err)
				}
				for _, l := range linkRows {
					_ = cw.Write([]string{
						strconv.FormatUint(uint64(l.ID), 10),
						l.Host,
						l.Slug,
						l.Target,
						l.Title,
						strconv.FormatInt(l.Clicks, 10),
						l.Tags,
						l.CreatedAt.UTC().Format(time.RFC3339),
					})
				}
				cw.Flush()
			} else {
				slog.Warn("export: failed creating links.csv", "err", err)
			}

			// 3. domains.json
			if err := addJSON("domains.json", domainItems); err != nil {
				slog.Warn("export: failed writing domains.json", "err", err)
			}

			// 4. mailboxes.json
			if err := addJSON("mailboxes.json", mailboxItems); err != nil {
				slog.Warn("export: failed writing mailboxes.json", "err", err)
			}

			// 5. audit_logs.json
			if err := addJSON("audit_logs.json", auditLogs); err != nil {
				slog.Warn("export: failed writing audit_logs.json", "err", err)
			}

			// Supplementary mounted plugins export data
			for _, p := range h.plugins {
				if fn, ok := plugin.LookupServiceAs[plugin.ExportFunc](h.LookupService, plugin.ExportServiceName(p.Name())); ok {
					pluginData := fn(org)
					if len(pluginData) > 0 {
						_ = addJSON(fmt.Sprintf("plugins/%s.json", p.Name()), pluginData)
					}
				}
			}

			if err := zw.Close(); err != nil {
				slog.Error("export: error closing zip writer", "err", err)
			}
		},
	}, nil
}

package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func unzipTestArchive(t *testing.T, zipBytes []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("failed to open zip archive: %v", err)
	}
	files := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open zip entry %q: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed to read zip entry %q: %v", f.Name, err)
		}
		files[f.Name] = data
	}
	return files
}

type stubExportPlugin struct {
	name string
	fn   plugin.ExportFunc
}

func (p stubExportPlugin) Name() string          { return p.name }
func (p stubExportPlugin) Describe() plugin.Info { return plugin.Info{Title: p.name} }
func (p stubExportPlugin) Models() []any         { return nil }
func (p stubExportPlugin) Mount(m plugin.Mux, ctx *plugin.Context) {
	ctx.Provide(plugin.ExportServiceName(p.name), p.fn)
}

func TestExportZip_ContainsOnlyCoreAndPluginData(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 501
	const orgB uint = 502

	if err := db.Create(&models.Org{ID: orgA, Name: "Tenant Alpha", Slug: "tenant-a"}).Error; err != nil {
		t.Fatalf("seed org A: %v", err)
	}
	if err := db.Create(&models.Org{ID: orgB, Name: "Tenant Beta", Slug: "tenant-b"}).Error; err != nil {
		t.Fatalf("seed org B: %v", err)
	}

	userA := seedOrgMember(t, db, orgA, "admin@tenant-a.com", "admin")
	userB := seedOrgMember(t, db, orgB, "admin@tenant-b.com", "admin")
	userMember := seedOrgMember(t, db, orgA, "member@tenant-a.com", "member")

	db.Create(&testLink{OrgID: orgA, Slug: "alpha-docs", Target: "https://docs.alpha.com", Title: "Alpha Docs"})
	db.Create(&testDomain{OrgID: orgA, Name: "alpha.com", ZoneID: "zone-alpha-123"})
	db.Create(&testMailbox{OrgID: orgA, Address: "hello@alpha.com"})

	auditA := &models.AuditLog{OrgID: orgA, ActorID: userA, Action: "setting.update", TargetType: "setting", TargetID: 1, CreatedAt: time.Now().UTC().Add(-2 * time.Hour)}
	db.Create(auditA)
	auditB := &models.AuditLog{OrgID: orgB, ActorID: userB, Action: "org.secret", TargetType: "secret", TargetID: 999, CreatedAt: time.Now().UTC().Add(-1 * time.Hour)}
	db.Create(auditB)

	reg := plugin.NewRegistry()
	h.SetServiceLookup(reg.Lookup)
	fake := stubExportPlugin{
		name: "samplefeature",
		fn: func(orgID uint) map[string]any {
			return map[string]any{"exportedOrgId": orgID, "rows": []any{"row-a"}}
		},
	}
	h.SetPlugins([]plugin.Plugin{fake})
	pctx := &plugin.Context{Provide: reg.Provide}
	fake.Mount(nil, pctx)

	cookiesA := sessionCookies(t, userA, orgA)
	rec := do(srv, http.MethodGet, "/api/account/export", cookiesA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export request failed: code %d, body %s", rec.Code, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !contains(cd, "attachment") || !contains(cd, fmt.Sprintf("workspace-export-org%d-", orgA)) || !contains(cd, ".zip") {
		t.Errorf("Content-Disposition = %q, want attachment with org%d and .zip extension", cd, orgA)
	}
	if cc := rec.Header().Get("Cache-Control"); !contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	files := unzipTestArchive(t, rec.Body.Bytes())

	for _, expected := range []string{"metadata.json", "audit_logs.json", "plugins/samplefeature.json"} {
		if _, ok := files[expected]; !ok {
			t.Errorf("missing expected file in ZIP: %s (got %v)", expected, fileNames(files))
		}
	}
	for _, forbidden := range []string{"links.csv", "domains.json", "mailboxes.json"} {
		if _, ok := files[forbidden]; ok {
			t.Errorf("core export must not contain business file %q", forbidden)
		}
	}
	for name, data := range files {
		if contains(string(data), "alpha-docs") || contains(string(data), "hello@alpha.com") {
			t.Errorf("business table data leaked into %s", name)
		}
	}

	var meta map[string]any
	if err := json.Unmarshal(files["metadata.json"], &meta); err != nil {
		t.Fatalf("unmarshal metadata.json: %v", err)
	}
	if orgIDFloat, ok := meta["orgId"].(float64); !ok || uint(orgIDFloat) != orgA {
		t.Errorf("metadata orgId = %v, want %d", meta["orgId"], orgA)
	}
	if wsName, _ := meta["workspaceName"].(string); wsName != "Tenant Alpha" {
		t.Errorf("metadata workspaceName = %q, want Tenant Alpha", wsName)
	}
	filesList, ok := meta["files"].([]any)
	if !ok {
		t.Fatalf("metadata missing files list: %+v", meta)
	}
	wantFiles := []string{"metadata.json", "plugins/samplefeature.json", "audit_logs.json"}
	if len(filesList) != len(wantFiles) {
		t.Fatalf("metadata files = %v, want %v", filesList, wantFiles)
	}
	for i, want := range wantFiles {
		if filesList[i] != want {
			t.Errorf("metadata files[%d] = %v, want %q", i, filesList[i], want)
		}
	}
	counts, ok := meta["counts"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing counts map: %+v", meta)
	}
	if cnt, _ := counts["plugins"].(float64); int64(cnt) != 1 {
		t.Errorf("metadata counts.plugins = %v, want 1", counts["plugins"])
	}
	if cnt, _ := counts["auditLogs"].(float64); int64(cnt) < 1 {
		t.Errorf("metadata counts.auditLogs = %v, want at least 1", counts["auditLogs"])
	}
	for _, businessKey := range []string{"links", "domains", "mailboxes"} {
		if _, ok := counts[businessKey]; ok {
			t.Errorf("metadata counts must not contain business key %q", businessKey)
		}
	}
	metaText := string(files["metadata.json"])
	for _, businessName := range []string{"links.csv", "domains.json", "mailboxes.json"} {
		if contains(metaText, businessName) {
			t.Errorf("metadata must not mention %q: %s", businessName, metaText)
		}
	}

	var pluginPayload map[string]any
	if err := json.Unmarshal(files["plugins/samplefeature.json"], &pluginPayload); err != nil {
		t.Fatalf("unmarshal plugins/samplefeature.json: %v", err)
	}
	if got, _ := pluginPayload["exportedOrgId"].(float64); uint(got) != orgA {
		t.Errorf("plugin ExportFunc received orgID %v, want %d", pluginPayload["exportedOrgId"], orgA)
	}
	if rows, _ := pluginPayload["rows"].([]any); len(rows) != 1 {
		t.Errorf("plugin payload rows = %v, want one row", pluginPayload["rows"])
	}

	var auditData []models.AuditLog
	if err := json.Unmarshal(files["audit_logs.json"], &auditData); err != nil {
		t.Fatalf("unmarshal audit_logs.json: %v", err)
	}
	hasDataExportAudit := false
	for _, l := range auditData {
		if l.OrgID != orgA {
			t.Errorf("CROSS-TENANT LEAK: audit log entry has OrgID = %d, want %d", l.OrgID, orgA)
		}
		if l.Action == "org.secret" {
			t.Errorf("CROSS-TENANT LEAK: tenant B audit action present in tenant A export")
		}
		if l.Action == "account.data_export" {
			hasDataExportAudit = true
		}
	}
	if !hasDataExportAudit {
		t.Error("FAIL-CLOSED AUDIT MISSING: account.data_export audit event was not recorded in audit_logs")
	}

	cookiesMember := sessionCookies(t, userMember, orgA)
	recMember := do(srv, http.MethodGet, "/api/account/export", cookiesMember, "")
	if recMember.Code != http.StatusForbidden {
		t.Errorf("member export status = %d, want 403 Forbidden", recMember.Code)
	}

	recUnauth := do(srv, http.MethodGet, "/api/account/export", nil, "")
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("unauth export status = %d, want 401 Unauthorized", recUnauth.Code)
	}

	signedURL, err := h.GenerateExportSignedURL(orgA)
	if err != nil {
		t.Fatalf("GenerateExportSignedURL failed: %v", err)
	}
	recSigned := do(srv, http.MethodGet, signedURL, nil, "")
	if recSigned.Code != http.StatusOK {
		t.Fatalf("signed token export status = %d, want 200 OK (%s)", recSigned.Code, recSigned.Body.String())
	}
	if ct := recSigned.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("signed download Content-Type = %q, want application/zip", ct)
	}

	recBadToken := do(srv, http.MethodGet, "/api/account/export?token=invalid-garbage-token", nil, "")
	if recBadToken.Code != http.StatusUnauthorized {
		t.Errorf("invalid token export status = %d, want 401 Unauthorized", recBadToken.Code)
	}
}

func fileNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	return names
}

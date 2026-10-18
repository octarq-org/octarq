package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	dnsmodels "github.com/octarq-org/octarq/server/plugins/dns"
	linksmodels "github.com/octarq-org/octarq/server/plugins/links"
	mailmodels "github.com/octarq-org/octarq/server/plugins/mail"
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

type mockExportDNSManager struct {
	records map[uint][]plugin.DNSRecord
}

func (m *mockExportDNSManager) List(ctx context.Context, orgID, domainID uint) ([]plugin.DNSRecord, error) {
	return m.records[domainID], nil
}

func (m *mockExportDNSManager) Set(ctx context.Context, orgID, domainID uint, r plugin.DNSRecord) (plugin.DNSRecord, error) {
	return r, nil
}

func (m *mockExportDNSManager) Delete(ctx context.Context, orgID, domainID uint, recordID string) error {
	return nil
}

func TestExportZip_CompleteDataPortabilityAndCrossOrgIsolation(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 501
	const orgB uint = 502

	// Seed organizations
	db.Create(&models.Org{ID: orgA, Name: "Tenant Alpha", Slug: "tenant-a"})
	db.Create(&models.Org{ID: orgB, Name: "Tenant Beta", Slug: "tenant-b"})

	// Seed users and members
	userA := seedOrgMember(t, db, orgA, "admin@tenant-a.com", "admin")
	userB := seedOrgMember(t, db, orgB, "admin@tenant-b.com", "admin")
	userMember := seedOrgMember(t, db, orgA, "member@tenant-a.com", "member")

	// Seed Tenant A resources
	linkA1 := &linksmodels.Link{OrgID: orgA, Slug: "alpha-docs", Target: "https://docs.alpha.com", Title: "Alpha Docs", Clicks: 42, Tags: "docs,alpha"}
	linkA2 := &linksmodels.Link{OrgID: orgA, Slug: "alpha-app", Target: "https://app.alpha.com", Title: "Alpha App", Clicks: 108, Tags: "app"}
	linkA3 := &linksmodels.Link{OrgID: orgA, Slug: "alpha-blog", Target: "https://blog.alpha.com", Title: "Alpha Blog", Clicks: 5, Tags: "blog"}
	db.Create(linkA1)
	db.Create(linkA2)
	db.Create(linkA3)

	domA := &dnsmodels.Domain{OrgID: orgA, Name: "alpha.com", ZoneID: "zone-alpha-123"}
	db.Create(domA)

	mbA1 := &mailmodels.Mailbox{OrgID: orgA, Address: "hello@alpha.com", Note: "General inquiries"}
	mbA2 := &mailmodels.Mailbox{OrgID: orgA, Address: "billing@alpha.com", Note: "Billing invoices"}
	db.Create(mbA1)
	db.Create(mbA2)
	db.Create(&mailmodels.Email{MailboxID: mbA1.ID, FromAddr: "sender@example.com", Subject: "Hi Alpha", Read: true})
	db.Create(&mailmodels.Email{MailboxID: mbA1.ID, FromAddr: "sender2@example.com", Subject: "Unread Msg", Read: false})

	auditA := &models.AuditLog{OrgID: orgA, ActorID: userA, Action: "domain.create", TargetType: "domain", TargetID: domA.ID, CreatedAt: time.Now().UTC().Add(-2 * time.Hour)}
	db.Create(auditA)

	// Seed Tenant B resources (to verify cross-tenant isolation)
	linkB1 := &linksmodels.Link{OrgID: orgB, Slug: "beta-secret", Target: "https://secret.beta.com", Title: "Beta Secret", Clicks: 999, Tags: "beta"}
	linkB2 := &linksmodels.Link{OrgID: orgB, Slug: "beta-dashboard", Target: "https://dash.beta.com", Title: "Beta Dash", Clicks: 12}
	db.Create(linkB1)
	db.Create(linkB2)

	domB := &dnsmodels.Domain{OrgID: orgB, Name: "beta-private.com", ZoneID: "zone-beta-999"}
	db.Create(domB)

	mbB := &mailmodels.Mailbox{OrgID: orgB, Address: "stealth@beta-private.com", Note: "Confidential"}
	db.Create(mbB)

	auditB := &models.AuditLog{OrgID: orgB, ActorID: userB, Action: "org.secret", TargetType: "secret", TargetID: 999, CreatedAt: time.Now().UTC().Add(-1 * time.Hour)}
	db.Create(auditB)

	// Register DNS mock with DNS records including a secret that must be masked
	const rawSecretToken = "sk-live-secret-key-that-must-be-redacted"
	mockDNS := &mockExportDNSManager{
		records: map[uint][]plugin.DNSRecord{
			domA.ID: {
				{ID: "rec-1", Type: "A", Name: "app.alpha.com", Content: "1.2.3.4", TTL: 300},
				{ID: "rec-2", Type: "TXT", Name: "_verify.alpha.com", Content: "token=" + rawSecretToken, TTL: 300},
			},
		},
	}
	reg := plugin.NewRegistry()
	reg.Provide(plugin.ServiceDNSManager, plugin.DNSManager(mockDNS))
	h.SetServiceLookup(reg.Lookup)

	// 1. Request export as Tenant A admin
	cookiesA := sessionCookies(t, userA, orgA)
	rec := do(srv, http.MethodGet, "/api/account/export", cookiesA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export request failed: code %d, body %s", rec.Code, rec.Body.String())
	}

	// 2. Validate HTTP headers
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !contains(cd, "attachment") || !contains(cd, fmt.Sprintf("workspace-export-org%d-", orgA)) || !contains(cd, ".zip") {
		t.Errorf("Content-Disposition = %q, want attachment with org%d and .zip extension", cd, orgA)
	}
	if cc := rec.Header().Get("Cache-Control"); !contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	// 3. Unzip and verify structure
	files := unzipTestArchive(t, rec.Body.Bytes())

	expectedFiles := []string{
		"metadata.json",
		"links.csv",
		"domains.json",
		"mailboxes.json",
		"audit_logs.json",
	}
	for _, expected := range expectedFiles {
		if _, ok := files[expected]; !ok {
			t.Errorf("missing expected file in ZIP: %s", expected)
		}
	}

	// 4. Verify metadata.json
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
	counts, ok := meta["counts"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing counts map: %+v", meta)
	}
	if cnt, _ := counts["links"].(float64); int64(cnt) != 3 {
		t.Errorf("metadata counts.links = %v, want 3", cnt)
	}
	if cnt, _ := counts["domains"].(float64); int64(cnt) != 1 {
		t.Errorf("metadata counts.domains = %v, want 1", cnt)
	}
	if cnt, _ := counts["mailboxes"].(float64); int64(cnt) != 2 {
		t.Errorf("metadata counts.mailboxes = %v, want 2", cnt)
	}

	// 5. Verify links.csv rows and strict isolation
	csvReader := csv.NewReader(bytes.NewReader(files["links.csv"]))
	csvRecords, err := csvReader.ReadAll()
	if err != nil {
		t.Fatalf("read links.csv: %v", err)
	}
	if len(csvRecords) != 4 { // 1 header + 3 data rows
		t.Fatalf("links.csv row count = %d, want 4 (header + 3 links)", len(csvRecords))
	}
	expectedHeaders := []string{"ID", "Host", "Slug", "Target", "Title", "Clicks", "Tags", "CreatedAt"}
	for i, hName := range expectedHeaders {
		if csvRecords[0][i] != hName {
			t.Errorf("header col %d = %q, want %q", i, csvRecords[0][i], hName)
		}
	}
	linksCSVText := string(files["links.csv"])
	if !contains(linksCSVText, "alpha-docs") || !contains(linksCSVText, "alpha-app") || !contains(linksCSVText, "alpha-blog") {
		t.Errorf("links.csv missing expected links: %s", linksCSVText)
	}
	// Multi-tenant assertion: Tenant B links MUST NOT leak into Tenant A's export
	if contains(linksCSVText, "beta-secret") || contains(linksCSVText, "beta-dashboard") {
		t.Errorf("CROSS-TENANT LEAK: Tenant B link data present in Tenant A export: %s", linksCSVText)
	}

	// 6. Verify domains.json and secret masking
	var domainsData []map[string]any
	if err := json.Unmarshal(files["domains.json"], &domainsData); err != nil {
		t.Fatalf("unmarshal domains.json: %v", err)
	}
	if len(domainsData) != 1 {
		t.Fatalf("domains count = %d, want 1", len(domainsData))
	}
	if domainsData[0]["name"] != "alpha.com" {
		t.Errorf("domain name = %v, want alpha.com", domainsData[0]["name"])
	}
	// Assert Tenant B domain is NOT present
	domainsJSONText := string(files["domains.json"])
	if contains(domainsJSONText, "beta-private.com") {
		t.Errorf("CROSS-TENANT LEAK: Tenant B domain present in Tenant A export")
	}
	// Assert Secret Masking: raw secret must NOT exist, must be masked as sk-****
	if contains(domainsJSONText, rawSecretToken) {
		t.Errorf("SECURITY LEAK: raw DNS secret token %q leaked in export", rawSecretToken)
	}
	if !contains(domainsJSONText, "sk-****") {
		t.Errorf("expected secret to be masked with sk-****: %s", domainsJSONText)
	}

	// 7. Verify mailboxes.json and stats
	var mailboxesData []map[string]any
	if err := json.Unmarshal(files["mailboxes.json"], &mailboxesData); err != nil {
		t.Fatalf("unmarshal mailboxes.json: %v", err)
	}
	if len(mailboxesData) != 2 {
		t.Fatalf("mailboxes count = %d, want 2", len(mailboxesData))
	}
	mailboxesJSONText := string(files["mailboxes.json"])
	if !contains(mailboxesJSONText, "hello@alpha.com") || !contains(mailboxesJSONText, "billing@alpha.com") {
		t.Errorf("mailboxes.json missing expected mailboxes: %s", mailboxesJSONText)
	}
	if contains(mailboxesJSONText, "stealth@beta-private.com") {
		t.Errorf("CROSS-TENANT LEAK: Tenant B mailbox present in Tenant A export")
	}

	// 8. Verify audit_logs.json and Fail-Closed audit entry
	var auditData []models.AuditLog
	if err := json.Unmarshal(files["audit_logs.json"], &auditData); err != nil {
		t.Fatalf("unmarshal audit_logs.json: %v", err)
	}
	hasDataExportAudit := false
	for _, l := range auditData {
		if l.OrgID != orgA {
			t.Errorf("CROSS-TENANT LEAK: audit log entry has OrgID = %d, want %d", l.OrgID, orgA)
		}
		if l.Action == "account.data_export" {
			hasDataExportAudit = true
		}
	}
	if !hasDataExportAudit {
		t.Error("FAIL-CLOSED AUDIT MISSING: account.data_export audit event was not recorded in audit_logs")
	}

	// 9. Verify role access controls: member role -> 403 Forbidden
	cookiesMember := sessionCookies(t, userMember, orgA)
	recMember := do(srv, http.MethodGet, "/api/account/export", cookiesMember, "")
	if recMember.Code != http.StatusForbidden {
		t.Errorf("member export status = %d, want 403 Forbidden", recMember.Code)
	}

	// 10. Verify unauthenticated -> 401 Unauthorized
	recUnauth := do(srv, http.MethodGet, "/api/account/export", nil, "")
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("unauth export status = %d, want 401 Unauthorized", recUnauth.Code)
	}

	// 11. Verify 15-minute temporary signed token download
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

	// 12. Invalid token -> 401 Unauthorized
	recBadToken := do(srv, http.MethodGet, "/api/account/export?token=invalid-garbage-token", nil, "")
	if recBadToken.Code != http.StatusUnauthorized {
		t.Errorf("invalid token export status = %d, want 401 Unauthorized", recBadToken.Code)
	}
}

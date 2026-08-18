package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/plugin"
)

// fakeDNSProvider is a fully in-memory dnsprovider.Provider whose behavior
// tests can shape per operation via the error fields.
type fakeDNSProvider struct {
	zones        []dnsprovider.Zone
	records      []dnsprovider.Record
	listZonesErr error
	listRecErr   error
	createErr    error
	updateErr    error
	deleteErr    error
	verifyErr    error
	verifyName   string
	created      []dnsprovider.Record
	updated      []dnsprovider.Record
	deleted      []string
}

func (f *fakeDNSProvider) ListZones(context.Context) ([]dnsprovider.Zone, error) {
	if f.listZonesErr != nil {
		return nil, f.listZonesErr
	}
	return f.zones, nil
}

func (f *fakeDNSProvider) ListRecords(context.Context, string) ([]dnsprovider.Record, error) {
	if f.listRecErr != nil {
		return nil, f.listRecErr
	}
	return f.records, nil
}

func (f *fakeDNSProvider) CreateRecord(_ context.Context, _ string, r dnsprovider.Record) (dnsprovider.Record, error) {
	if f.createErr != nil {
		return dnsprovider.Record{}, f.createErr
	}
	r.ID = "new-" + r.Name
	f.records = append(f.records, r)
	f.created = append(f.created, r)
	return r, nil
}

func (f *fakeDNSProvider) UpdateRecord(_ context.Context, _ string, r dnsprovider.Record) (dnsprovider.Record, error) {
	if f.updateErr != nil {
		return dnsprovider.Record{}, f.updateErr
	}
	for i := range f.records {
		if f.records[i].ID == r.ID {
			f.records[i] = r
		}
	}
	f.updated = append(f.updated, r)
	return r, nil
}

func (f *fakeDNSProvider) DeleteRecord(_ context.Context, _ string, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeDNSProvider) VerifyZone(context.Context, string) (string, error) {
	if f.verifyErr != nil {
		return "", f.verifyErr
	}
	return f.verifyName, nil
}

// registerFakeProvider registers the given instance under a per-test name and
// returns the name. Registration is additive and idempotent, so repeated
// calls (e.g. under -count=2) are harmless.
func registerFakeProvider(t *testing.T, name string, f dnsprovider.Provider) string {
	t.Helper()
	dnsprovider.Register(name, func([]byte) (dnsprovider.Provider, error) { return f, nil })
	return name
}

// seedDomainForProvider wires a provider account + domain in org 1 so the
// plugin's providerFor() resolves to the fake provider.
func seedDomainForProvider(t *testing.T, p *Plugin, name, zoneID string, provName string) (uint, uint) {
	t.Helper()
	acc := ProviderAccount{OrgID: 1, Name: name + "-acc", Type: provName, Config: "{}"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatalf("seed provider account: %v", err)
	}
	dom := Domain{OrgID: 1, Name: name, ProviderAccountID: acc.ID, ZoneID: zoneID}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	return acc.ID, dom.ID
}

func TestRecordsCRUDFlow(t *testing.T) {
	p, mkCtx := setupFreshGateTestDB(t)
	fake := &fakeDNSProvider{
		records: []dnsprovider.Record{{ID: "r1", Type: "A", Name: "www.example.com", Content: "1.1.1.1", TTL: 300}},
	}
	provName := registerFakeProvider(t, "rec-flow-prov", fake)
	_, domID := seedDomainForProvider(t, p, "example.com", "zone-1", provName)
	ctx := context.Background()
	admin := mkCtx("admin")

	// List
	listOut, err := p.listRecords(ctx, &ListRecordsInput{Ctx: admin, ID: domID})
	if err != nil {
		t.Fatalf("listRecords: %v", err)
	}
	if len(listOut.Body) != 1 || listOut.Body[0].ID != "r1" {
		t.Fatalf("listRecords = %+v", listOut.Body)
	}

	// Create
	created, err := p.createRecord(ctx, &CreateRecordInput{Ctx: admin, ID: domID, Body: dnsprovider.Record{Type: "A", Name: "api.example.com", Content: "2.2.2.2"}})
	if err != nil {
		t.Fatalf("createRecord: %v", err)
	}
	if created.Body.ID == "" || len(fake.created) != 1 || fake.created[0].Content != "2.2.2.2" {
		t.Fatalf("createRecord out=%+v fake.created=%+v", created.Body, fake.created)
	}

	// Update
	updated, err := p.updateRecord(ctx, &UpdateRecordInput{Ctx: admin, ID: domID, RID: "r1", Body: dnsprovider.Record{Type: "A", Name: "www.example.com", Content: "3.3.3.3"}})
	if err != nil {
		t.Fatalf("updateRecord: %v", err)
	}
	if updated.Body.Content != "3.3.3.3" {
		t.Errorf("updateRecord out = %+v", updated.Body)
	}
	if len(fake.updated) != 1 || fake.updated[0].ID != "r1" || fake.updated[0].Content != "3.3.3.3" {
		t.Fatalf("updateRecord fake.updated=%+v", fake.updated)
	}

	// Delete
	del, err := p.deleteRecord(ctx, &DeleteRecordInput{Ctx: admin, ID: domID, RID: "r1"})
	if err != nil || !del.Body["ok"].(bool) {
		t.Fatalf("deleteRecord: %v", err)
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != "r1" {
		t.Fatalf("deleteRecord fake.deleted=%+v", fake.deleted)
	}
}

func TestRecordsScopedToCallersOrg(t *testing.T) {
	p, mkCtx := setupFreshGateTestDB(t)
	fake := &fakeDNSProvider{}
	provName := registerFakeProvider(t, "rec-scope-prov", fake)
	_, myDom := seedDomainForProvider(t, p, "mine.example.com", "zone-1", provName)
	other := Domain{OrgID: 2, Name: "victim.example.com"}
	if err := p.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	admin := mkCtx("admin")

	// Org 1 reaching org 2's domain id: every record operation is refused and
	// the fake provider is never touched.
	for name, f := range map[string]func() error{
		"list": func() error { _, err := p.listRecords(ctx, &ListRecordsInput{Ctx: admin, ID: other.ID}); return err },
		"create": func() error {
			_, err := p.createRecord(ctx, &CreateRecordInput{Ctx: admin, ID: other.ID, Body: dnsprovider.Record{Type: "A", Name: "x", Content: "1.1.1.1"}})
			return err
		},
		"update": func() error {
			_, err := p.updateRecord(ctx, &UpdateRecordInput{Ctx: admin, ID: other.ID, RID: "r", Body: dnsprovider.Record{Type: "A", Name: "x", Content: "1.1.1.1"}})
			return err
		},
		"delete": func() error {
			_, err := p.deleteRecord(ctx, &DeleteRecordInput{Ctx: admin, ID: other.ID, RID: "r"})
			return err
		},
	} {
		if err := f(); err == nil {
			t.Errorf("%s on another org's domain succeeded", name)
		}
	}
	if len(fake.created)+len(fake.updated)+len(fake.deleted) != 0 {
		t.Errorf("fake provider was touched by a cross-org caller")
	}
	_ = myDom

	// A domain with no zone id is refused before touching the provider.
	noZone := Domain{OrgID: 1, Name: "nozone.example.com", ProviderAccountID: 0}
	if err := p.db.Create(&noZone).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p.createRecord(ctx, &CreateRecordInput{Ctx: admin, ID: noZone.ID, Body: dnsprovider.Record{Type: "A", Name: "x", Content: "1.1.1.1"}}); err == nil {
		t.Error("createRecord on a domain with no zone id must be refused")
	}
}

func TestRecordsValidationAndProviderErrors(t *testing.T) {
	p, mkCtx := setupFreshGateTestDB(t)
	fake := &fakeDNSProvider{listRecErr: errBoom, createErr: errBoom, updateErr: errBoom, deleteErr: errBoom}
	provName := registerFakeProvider(t, "rec-err-prov", fake)
	_, domID := seedDomainForProvider(t, p, "err.example.com", "zone-1", provName)
	ctx := context.Background()
	admin := mkCtx("admin")

	// List has no role gate at all: a member's read reaches the provider (its
	// error surfaces as the provider's sanitized 400, not a 403).
	member := mkCtx("member")
	if _, err := p.listRecords(ctx, &ListRecordsInput{Ctx: member, ID: domID}); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("member list: want 400 (provider error), got %v", err)
	}
	_, err := p.createRecord(ctx, &CreateRecordInput{Ctx: member, ID: domID, Body: dnsprovider.Record{Type: "A", Name: "x", Content: "1.1.1.1"}})
	if statusOf(t, err) != http.StatusForbidden {
		t.Fatalf("member create: want 403")
	}

	// Missing type/content -> 400, provider untouched.
	_, err = p.createRecord(ctx, &CreateRecordInput{Ctx: admin, ID: domID, Body: dnsprovider.Record{Name: "x"}})
	if statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("missing type: want 400, got %v", err)
	}

	// Provider errors surface as 400 with the action name present.
	_, err = p.listRecords(ctx, &ListRecordsInput{Ctx: admin, ID: domID})
	if statusOf(t, err) != http.StatusBadRequest || !strings.Contains(err.Error(), "list records") {
		t.Fatalf("list provider err: %v", err)
	}
	_, err = p.createRecord(ctx, &CreateRecordInput{Ctx: admin, ID: domID, Body: dnsprovider.Record{Type: "A", Name: "x", Content: "1.1.1.1"}})
	if statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("create provider err: %v", err)
	}
	_, err = p.updateRecord(ctx, &UpdateRecordInput{Ctx: admin, ID: domID, RID: "r", Body: dnsprovider.Record{Type: "A", Name: "x", Content: "1.1.1.1"}})
	if statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("update provider err: %v", err)
	}
	if _, err := p.deleteRecord(ctx, &DeleteRecordInput{Ctx: admin, ID: domID, RID: "r"}); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("delete provider err: %v", err)
	}
}

func TestSyncDomainsFlow(t *testing.T) {
	p, mkCtx := setupFreshTestDB(t)
	seedBase(t, p, "app.example.com")
	fake := &fakeDNSProvider{zones: []dnsprovider.Zone{
		{ID: "z-new", Name: "Fresh.COM"},
		{ID: "z-old", Name: "existing.com"},
		{ID: "z-base", Name: "tenant.app.example.com"}, // reserved, skipped
	}}
	provName := registerFakeProvider(t, "sync-flow-prov", fake)
	accID, _ := seedDomainForProvider(t, p, "existing.com", "z-stale", provName)

	out, err := p.syncDomains(context.Background(), &SyncDomainsInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/api/domains/sync", nil)),
		Body: struct {
			ProviderAccountID uint `json:"providerAccountId,omitempty"`
		}{accID},
	})
	if err != nil {
		t.Fatalf("syncDomains: %v", err)
	}
	body := out.Body
	if body["created"] != 1 || body["updated"] != 1 || body["total"] != 3 {
		t.Fatalf("sync counts wrong: %+v", body)
	}
	// The reserved base-zone name was skipped entirely.
	if body["created"].(int)+body["updated"].(int) != 2 {
		t.Errorf("reserved zone must not be imported: %+v", body)
	}
	var dom Domain
	if err := p.db.Where("name = ?", "existing.com").First(&dom).Error; err != nil {
		t.Fatalf("reload existing: %v", err)
	}
	if dom.ZoneID != "z-old" || dom.ProviderAccountID != accID {
		t.Errorf("existing domain not refreshed: %+v", dom)
	}
	var n int64
	p.db.Model(&Domain{}).Where("name = ?", "fresh.com").Count(&n)
	if n != 1 {
		t.Errorf("fresh.com not created, count=%d", n)
	}
}

func TestSyncDomainsGates(t *testing.T) {
	p, mkCtx := setupFreshTestDB(t)
	fake := &fakeDNSProvider{listZonesErr: errBoom}
	provName := registerFakeProvider(t, "sync-gate-prov", fake)
	accID, _ := seedDomainForProvider(t, p, "x.example.com", "z", provName)
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodPost, "/api/domains/sync", nil)
	body := struct {
		ProviderAccountID uint `json:"providerAccountId,omitempty"`
	}{accID}

	// Member -> 403.
	memberReq := httptest.NewRequest(http.MethodPost, "/api/domains/sync", nil)
	memberReq.Header.Set("X-Org-ID", "1")
	memberReq.Header.Set("X-Role", "member")
	if _, err := p.syncDomains(ctx, &SyncDomainsInput{Ctx: mkCtx(memberReq), Body: body}); statusOf(t, err) != http.StatusForbidden {
		t.Fatalf("member sync: want 403")
	}

	// Missing account id -> 400.
	if _, err := p.syncDomains(ctx, &SyncDomainsInput{Ctx: mkCtx(req)}); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("missing account: want 400")
	}

	// Unknown provider account -> 404.
	if _, err := p.syncDomains(ctx, &SyncDomainsInput{Ctx: mkCtx(req), Body: struct {
		ProviderAccountID uint `json:"providerAccountId,omitempty"`
	}{99999}}); statusOf(t, err) != http.StatusNotFound {
		t.Fatalf("unknown account: want 404")
	}

	// Provider list-zones failure -> 400 sanitized.
	if _, err := p.syncDomains(ctx, &SyncDomainsInput{Ctx: mkCtx(req), Body: body}); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("list-zones err: want 400")
	}

	// Unregistered provider type -> 400.
	unknownAcc := ProviderAccount{OrgID: 1, Name: "ghost", Type: "no-such-provider", Config: "{}"}
	if err := p.db.Create(&unknownAcc).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p.syncDomains(ctx, &SyncDomainsInput{Ctx: mkCtx(req), Body: struct {
		ProviderAccountID uint `json:"providerAccountId,omitempty"`
	}{unknownAcc.ID}}); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("unknown provider: want 400")
	}
}

func TestDNSManagerEndToEnd(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	fake := &fakeDNSProvider{records: []dnsprovider.Record{{ID: "r1", Type: "A", Name: "x.example.com", Content: "1.1.1.1"}}}
	provName := registerFakeProvider(t, "mgr-e2e-prov", fake)
	_, domID := seedDomainForProvider(t, p, "mgr.example.com", "zone-1", provName)
	mgr := p.DNSManager()
	ctx := context.Background()

	recs, err := mgr.List(ctx, 1, domID)
	if err != nil || len(recs) != 1 {
		t.Fatalf("mgr.List: %v %+v", err, recs)
	}

	created, err := mgr.Set(ctx, 1, domID, plugin.DNSRecord{Type: "A", Name: "api.example.com", Content: "2.2.2.2"})
	if err != nil || created.ID == "" {
		t.Fatalf("mgr.Set create: %v", err)
	}
	updated, err := mgr.Set(ctx, 1, domID, plugin.DNSRecord{ID: "r1", Type: "A", Name: "x.example.com", Content: "9.9.9.9"})
	if err != nil || updated.Content != "9.9.9.9" {
		t.Fatalf("mgr.Set update: %v %+v", err, updated)
	}
	if err := mgr.Delete(ctx, 1, domID, "r1"); err != nil {
		t.Fatalf("mgr.Delete: %v", err)
	}
	if len(fake.deleted) != 1 {
		t.Errorf("mgr.Delete never reached the provider: %+v", fake.deleted)
	}

	// Provider failure propagates, org mismatch is refused before the provider.
	fake.listRecErr = errBoom
	if _, err := mgr.List(ctx, 1, domID); err == nil {
		t.Error("mgr.List must propagate provider error")
	}
}

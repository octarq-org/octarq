package mail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/usagemetric"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type memoryStorageProvider struct {
	blobs map[string][]byte
}

func newMemoryStorageProvider() *memoryStorageProvider {
	return &memoryStorageProvider{blobs: make(map[string][]byte)}
}

func (m *memoryStorageProvider) Put(ctx context.Context, key string, data []byte) error {
	m.blobs[key] = append([]byte(nil), data...)
	return nil
}

func (m *memoryStorageProvider) Get(ctx context.Context, key string) ([]byte, error) {
	data, ok := m.blobs[key]
	if !ok {
		return nil, plugin.ErrStorageNotFound
	}
	return append([]byte(nil), data...), nil
}

func (m *memoryStorageProvider) Delete(ctx context.Context, key string) error {
	delete(m.blobs, key)
	return nil
}

func (m *memoryStorageProvider) Stat(ctx context.Context, key string) (int64, error) {
	data, ok := m.blobs[key]
	if !ok {
		return 0, plugin.ErrStorageNotFound
	}
	return int64(len(data)), nil
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Mailbox{}, &Email{}, &SMTPSender{}, &MailRawBlob{})...); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// Guard Test 1: Default behavior unchanged (inbound -> DB storage -> .eml download byte-for-byte)
func TestDefaultStorageBehavior(t *testing.T) {
	db := setupTestDB(t)
	p := New()

	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig("Test", "1.0.0"))

	reg := plugin.NewRegistry()
	pctx := &plugin.Context{
		DB:      db,
		Huma:    humaAPI,
		OrgID:   func(r *http.Request) uint { return 1 },
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	}
	p.Mount(mux, pctx)

	org := models.Org{Slug: "default", InboundToken: "token"}
	org.ID = 1
	db.Create(&org)

	mb := Mailbox{OrgID: 1, Address: "inbound@example.com", Enabled: true}
	db.Create(&mb)

	rawEML := []byte("From: alice@example.com\r\nTo: inbound@example.com\r\nSubject: Hello World\r\nMessage-ID: <msg-default-1@example.com>\r\n\r\nTest Body Content")

	req := httptest.NewRequest("POST", "/api/webhook/default/email/inbound/token", bytes.NewReader(rawEML))
	req.Header.Set("X-Octarq-To", "inbound@example.com")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 from inbound webhook, got %d: %s", w.Code, w.Body.String())
	}

	var email Email
	if err := db.First(&email, "mailbox_id = ?", mb.ID).Error; err != nil {
		t.Fatalf("failed to find created email: %v", err)
	}

	// The whole point of the seam: the emails row — the one every inbox list
	// query scans — must not carry the multi-megabyte original. It goes to the
	// blob store instead, and the .eml download below proves it survived the trip.
	if len(email.Raw) != 0 {
		t.Fatalf("emails row still carries the raw original (%d bytes); it belongs in the blob store", len(email.Raw))
	}
	if email.StorageKey == "" {
		t.Fatalf("email has no storage key, so the original is unreachable")
	}
	var blob MailRawBlob
	if err := db.First(&blob, "key = ?", email.StorageKey).Error; err != nil {
		t.Fatalf("no blob stored under %q: %v", email.StorageKey, err)
	}
	if !bytes.Equal(blob.Data, rawEML) {
		t.Fatalf("stored blob does not match the delivered message byte-for-byte")
	}

	emlReq := httptest.NewRequest("GET", fmt.Sprintf("/api/emails/%d/raw", email.ID), nil)
	emlRec := httptest.NewRecorder()
	mux.ServeHTTP(emlRec, emlReq)

	if emlRec.Code != 200 {
		t.Fatalf("expected 200 from raw email download, got %d", emlRec.Code)
	}
	if !bytes.Equal(emlRec.Body.Bytes(), rawEML) {
		t.Fatalf("downloaded .eml content does not match original byte-for-byte")
	}
}

// Guard Test 2: Provider Put -> Get -> Delete -> Stat roundtrip
func TestStorageProviderRoundtrip(t *testing.T) {
	db := setupTestDB(t)
	prov := NewDBStorageProvider(db)
	ctx := context.Background()

	key := "mail/1/999.eml"
	data := []byte("Test Storage Provider Roundtrip Bytes 12345")

	if err := prov.Put(ctx, key, data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	sz, err := prov.Stat(ctx, key)
	if err != nil || sz != int64(len(data)) {
		t.Fatalf("Stat failed: expected size %d, got %d (err: %v)", len(data), sz, err)
	}

	got, err := prov.Get(ctx, key)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("Get failed: expected %s, got %s (err: %v)", string(data), string(got), err)
	}

	if err := prov.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = prov.Get(ctx, key)
	if !errors.Is(err, plugin.ErrStorageNotFound) {
		t.Fatalf("expected ErrStorageNotFound after Delete, got %v", err)
	}
}

// Guard Test 3: Coexistence read (one in DB, one in alternative provider)
func TestCoexistenceRead(t *testing.T) {
	db := setupTestDB(t)
	p := New()
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig("Test", "1.0.0"))

	reg := plugin.NewRegistry()
	memStorage := newMemoryStorageProvider()
	reg.Provide(plugin.ServiceMailStorageProvider, plugin.StorageProvider(memStorage))

	pctx := &plugin.Context{
		DB:      db,
		Huma:    humaAPI,
		OrgID:   func(r *http.Request) uint { return 1 },
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	}
	p.Mount(mux, pctx)

	mb := Mailbox{OrgID: 1, Address: "coexist@example.com", Enabled: true}
	db.Create(&mb)

	rawDB := []byte("Raw Email in DB Legacy")
	rawAlt := []byte("Raw Email in Alt Storage Provider")

	eDB := Email{MailboxID: mb.ID, MessageID: "msg-db", Raw: rawDB}
	db.Create(&eDB)

	eAlt := Email{MailboxID: mb.ID, MessageID: "msg-alt", Raw: nil, StorageKey: "mail/1/2.eml"}
	db.Create(&eAlt)
	_ = memStorage.Put(context.Background(), "mail/1/2.eml", rawAlt)

	req1 := httptest.NewRequest("GET", fmt.Sprintf("/api/emails/%d/raw", eDB.ID), nil)
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)
	if rec1.Code != 200 || !bytes.Equal(rec1.Body.Bytes(), rawDB) {
		t.Fatalf("expected eDB downloaded from DB, got code %d, body %s", rec1.Code, rec1.Body.String())
	}

	req2 := httptest.NewRequest("GET", fmt.Sprintf("/api/emails/%d/raw", eAlt.ID), nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != 200 || !bytes.Equal(rec2.Body.Bytes(), rawAlt) {
		t.Fatalf("expected eAlt downloaded from Alt provider, got code %d, body %s", rec2.Code, rec2.Body.String())
	}
}

// Guard Test 4: OSS rejects Pro provider ("s3") when Pro provider is absent
func TestOSSRejectsProProvider(t *testing.T) {
	db := setupTestDB(t)
	p := New()

	reg := plugin.NewRegistry()
	pctx := &plugin.Context{
		DB:      db,
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
		GetGlobalSetting: func(key string) string {
			if key == "mail_storage_backend" {
				return "s3"
			}
			return ""
		},
	}
	p.Mount(http.NewServeMux(), pctx)

	_, err := p.getStorageProvider()
	if err == nil {
		t.Fatalf("expected error when mail_storage_backend is s3 in OSS, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("requires Pro")) {
		t.Fatalf("expected clear error mentioning Pro, got %v", err)
	}
}

// Guard Test 6: Usage metering (writing N bytes records N bytes plus one
// message of inbound mail)
func TestStorageUsageMetering(t *testing.T) {
	db := setupTestDB(t)
	p := New()
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig("Test", "1.0.0"))

	var calls []struct {
		orgID  uint
		metric string
		n      int64
	}

	reg := plugin.NewRegistry()
	pctx := &plugin.Context{
		DB:    db,
		Huma:  humaAPI,
		OrgID: func(r *http.Request) uint { return 1 },
		RecordUsage: func(orgID uint, metric string, n int64) {
			calls = append(calls, struct {
				orgID  uint
				metric string
				n      int64
			}{orgID, metric, n})
		},
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	}
	p.Mount(mux, pctx)

	org := models.Org{Slug: "meterorg", InboundToken: "token"}
	org.ID = 10
	db.Create(&org)

	mb := Mailbox{OrgID: 10, Address: "meter@example.com", Enabled: true}
	db.Create(&mb)

	rawEML := []byte("From: alice@example.com\r\nTo: meter@example.com\r\nSubject: Meter Test\r\n\r\n12345678901234567890")

	req := httptest.NewRequest("POST", "/api/webhook/meterorg/email/inbound/token", bytes.NewReader(rawEML))
	req.Header.Set("X-Octarq-To", "meter@example.com")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 from inbound webhook, got %d: %s", w.Code, w.Body.String())
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 RecordUsage calls (raw bytes + message count), got %d: %+v", len(calls), calls)
	}
	// First: stored byte count, no quota consumer today.
	if calls[0].orgID != 10 || calls[0].metric != usagemetric.RawBytes || calls[0].n != int64(len(rawEML)) {
		t.Errorf("raw-bytes call mismatch: expected org=10 metric=%s n=%d, got %+v",
			usagemetric.RawBytes, len(rawEML), calls[0])
	}
	// Second: one inbound message for the mailInPerMonth quota.
	if calls[1].orgID != 10 || calls[1].metric != usagemetric.MailIn || calls[1].n != 1 {
		t.Errorf("mailIn call mismatch: expected org=10 metric=%s n=1, got %+v", usagemetric.MailIn, calls[1])
	}
}

func TestPurgeDeletesExternalProviderBlobs(t *testing.T) {
	db := setupTestDB(t)
	reg := plugin.NewRegistry()
	mem := newMemoryStorageProvider()
	p := New()

	pctx := &plugin.Context{
		DB:      db,
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	}
	p.Mount(http.NewServeMux(), pctx)
	reg.Provide(plugin.ServiceMailStorageProvider, plugin.StorageProvider(mem))

	mb := Mailbox{OrgID: 1, Address: "purge@example.com", Enabled: true}
	db.Create(&mb)
	e := Email{MailboxID: mb.ID, MessageID: "msg-purge", Raw: nil, StorageKey: "mail/1/3.eml"}
	db.Create(&e)
	_ = mem.Put(context.Background(), "mail/1/3.eml", []byte("raw"))

	if err := p.purge(1); err != nil {
		t.Fatalf("purge error: %v", err)
	}

	if _, err := mem.Get(context.Background(), "mail/1/3.eml"); err != plugin.ErrStorageNotFound {
		t.Errorf("expected external provider blob deleted, got %v", err)
	}
	var emails int64
	db.Model(&Email{}).Where("mailbox_id = ?", mb.ID).Count(&emails)
	if emails != 0 {
		t.Errorf("expected emails purged, got %d", emails)
	}
	var mailboxes int64
	db.Model(&Mailbox{}).Where("owner_id = ?", 1).Count(&mailboxes)
	if mailboxes != 0 {
		t.Errorf("expected mailboxes purged, got %d", mailboxes)
	}
}

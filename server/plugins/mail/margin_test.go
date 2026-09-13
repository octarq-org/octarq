package mail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
)

func TestRawEmailDerivesKeyAndFallsBackToDBBlob(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "derived@example.com", Enabled: true}
	p.db.Create(&mb)
	e := Email{MailboxID: mb.ID, ReceivedAt: time.Now()}
	p.db.Create(&e)

	// No Raw, no StorageKey -> the derived key resolves from the database blob.
	dbProv := NewDBStorageProvider(p.db)
	key := fmt.Sprintf("mail/%d/%d.eml", mb.OrgID, e.ID)
	if err := dbProv.Put(ctx, key, []byte("derived blob")); err != nil {
		t.Fatalf("seed blob at derived key: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/emails/1/raw", nil)
	if _, err := p.rawEmail(ctx, &RawEmailInput{
		Ctx: humago.NewContext(nil, req, rec),
		ID:  e.ID,
	}); err != nil {
		t.Fatalf("rawEmail derived-key: %v", err)
	}
	if rec.Body.String() != "derived blob" {
		t.Errorf("body = %q, want derived blob", rec.Body.String())
	}
}

func TestPurgeFallsBackWhenStorageProviderUnavailable(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	// Force getStorageProvider to error (Pro backend with no registered
	// provider): purge must still delete the database rows and blobs.
	p.getGlobalSetting = func(key string) string {
		if key == "mail_storage_backend" {
			return "s3"
		}
		return ""
	}

	mb := Mailbox{OrgID: 1, Address: "pf@example.com", Enabled: true}
	p.db.Create(&mb)
	e := Email{MailboxID: mb.ID, ReceivedAt: time.Now()}
	p.db.Create(&e)
	dbProv := NewDBStorageProvider(p.db)
	if err := dbProv.Put(ctx, fmt.Sprintf("mail/1/%d.eml", e.ID), []byte("x")); err != nil {
		t.Fatalf("seed blob: %v", err)
	}

	if err := p.purge(1); err != nil {
		t.Fatalf("purge with unavailable provider: %v", err)
	}
	var boxes int64
	p.db.Model(&Mailbox{}).Where("owner_id = ?", 1).Count(&boxes)
	if boxes != 0 {
		t.Errorf("mailboxes after purge = %d, want 0", boxes)
	}
	var blobs int64
	p.db.Model(&MailRawBlob{}).Count(&blobs)
	if blobs != 0 {
		t.Errorf("db blobs after purge = %d, want 0", blobs)
	}
}

func TestSuppressionHandlersOrgZero(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Org-ID", "0")
	z := humago.NewContext(nil, r, httptest.NewRecorder())

	if _, err := p.listSuppressions(ctx, &ListSuppressionsInput{Ctx: z}); err == nil {
		t.Error("listSuppressions with org 0 must fail")
	}
	if _, err := p.createSuppression(ctx, &CreateSuppressionInput{Ctx: z, Body: suppressionDTO{Address: "a@b.c"}}); err == nil {
		t.Error("createSuppression with org 0 must fail")
	}
	if _, err := p.deleteSuppression(ctx, &DeleteSuppressionInput{Ctx: z, ID: 1}); err == nil {
		t.Error("deleteSuppression with org 0 must fail")
	}
}

func TestCreateMailboxCrossTenantDomainForbidden(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	// Another tenant owns victim.example as a mail host.
	p.db.Create(&dns.Domain{OrgID: 2, Name: "victim.example", ForMail: true,
		MailHosts: models.HostList{{Host: "victim.example", Enabled: true}}})

	req := httptest.NewRequest(http.MethodPost, "/api/mailboxes", nil)
	if _, err := p.createMailbox(ctx, &CreateMailboxInput{
		Ctx:  humago.NewContext(nil, req, httptest.NewRecorder()),
		Body: mailboxDTO{Address: "billing@victim.example"},
	}); err == nil {
		t.Error("creating a mailbox on another tenant's mail host must be forbidden")
	}
}

func TestDeleteMailboxRemovesEmails(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	mb := Mailbox{OrgID: 1, Address: "rem@example.com", Enabled: true}
	p.db.Create(&mb)
	p.db.Create(&Email{MailboxID: mb.ID, ReceivedAt: time.Now()})

	req := httptest.NewRequest(http.MethodDelete, "/api/mailboxes/1", nil)
	out, err := p.deleteMailbox(ctx, &DeleteMailboxInput{Ctx: humago.NewContext(nil, req, httptest.NewRecorder()), ID: mb.ID})
	if err != nil || !out.Body["ok"] {
		t.Fatalf("deleteMailbox: %v", err)
	}
	var emails int64
	p.db.Model(&Email{}).Count(&emails)
	if emails != 0 {
		t.Errorf("emails surviving mailbox delete = %d, want 0", emails)
	}
}

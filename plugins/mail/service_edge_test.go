package mail

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"fmt"

	"github.com/octarq-org/octarq/internal/usagemetric"
	"github.com/octarq-org/octarq/plugin"
)

func TestGetEmailForSummarize(t *testing.T) {
	p, _ := newBouncePlugin(t)
	db := p.db

	mb := Mailbox{OrgID: 1, Address: "sum@example.com", Enabled: true}
	db.Create(&mb)

	// Text-only email.
	textEmail := Email{MailboxID: mb.ID, FromAddr: "a@x.com", Subject: "s1", Text: "plain body", HTML: "<p>ht</p>", ReceivedAt: time.Now()}
	db.Create(&textEmail)
	from, subject, body, ok := p.getEmailForSummarize(1, textEmail.ID)
	if !ok || from != "a@x.com" || subject != "s1" || body != "plain body" {
		t.Errorf("text email: ok=%v from=%q subject=%q body=%q", ok, from, subject, body)
	}

	// HTML-only email strips tags.
	htmlEmail := Email{MailboxID: mb.ID, FromAddr: "b@x.com", Subject: "s2", Text: "", HTML: "<p>Hello <b>World</b></p>", ReceivedAt: time.Now()}
	db.Create(&htmlEmail)
	_, _, body2, ok2 := p.getEmailForSummarize(1, htmlEmail.ID)
	if !ok2 || !strings.Contains(body2, "Hello") || strings.Contains(body2, "<p>") {
		t.Errorf("html email: ok=%v body=%q", ok2, body2)
	}

	// Missing id -> not ok.
	if _, _, _, ok := p.getEmailForSummarize(1, 999); ok {
		t.Error("missing email must not resolve")
	}
	// Another org's email is invisible.
	mb2 := Mailbox{OrgID: 2, Address: "other@example.com", Enabled: true}
	db.Create(&mb2)
	otherEmail := Email{MailboxID: mb2.ID, FromAddr: "c@x.com", Subject: "s3", Text: "x", ReceivedAt: time.Now()}
	db.Create(&otherEmail)
	if _, _, _, ok := p.getEmailForSummarize(1, otherEmail.ID); ok {
		t.Error("cross-org email must not resolve")
	}
}

func TestSplitListAndIsReservedSlugMail(t *testing.T) {
	if got := splitList(""); got != nil {
		t.Errorf("splitList(\"\") = %v, want nil", got)
	}
	if got := splitList("a, b, ,c"); len(got) != 3 || got[1] != "b" {
		t.Errorf("splitList = %v", got)
	}

	p := &Plugin{}
	if !p.isReservedSlug("ADMIN") {
		t.Error("builtin reserved slug must be case-insensitive")
	}
	if p.isReservedSlug("free") {
		t.Error("ordinary slug must not be reserved")
	}
	p.getGlobalSetting = func(key string) string {
		if key == "reserved_slugs" {
			return "support, ops"
		}
		return ""
	}
	if !p.isReservedSlug("ops") || !p.isReservedSlug("SUPPORT") {
		t.Error("globally configured reserved slugs not honored")
	}
}

func TestMailReadyReflectsSenderPresence(t *testing.T) {
	p, _ := newBouncePlugin(t)
	if p.mailReady() {
		t.Fatal("mailReady must be false with no senders")
	}
	p.db.Create(&SMTPSender{OrgID: 1, Name: "relay", Host: "smtp.example.com", Port: 587})
	if !p.mailReady() {
		t.Fatal("mailReady must be true once a sender exists")
	}
}

func TestPurgeRemovesOrgData(t *testing.T) {
	p, _ := newBouncePlugin(t)
	db := p.db

	mb := Mailbox{OrgID: 1, Address: "purge@example.com", Enabled: true}
	db.Create(&mb)
	email := Email{MailboxID: mb.ID, FromAddr: "x@y.z", Subject: "bye", ReceivedAt: time.Now()}
	db.Create(&email)
	prov := NewDBStorageProvider(db)
	key := fmt.Sprintf("mail/1/%d.eml", email.ID)
	if err := prov.Put(context.Background(), key, []byte("raw")); err != nil {
		t.Fatalf("seed blob: %v", err)
	}
	// A second org's data must survive the purge.
	db.Create(&Mailbox{OrgID: 2, Address: "keep@example.com", Enabled: true})

	if err := p.purge(1); err != nil {
		t.Fatalf("purge: %v", err)
	}
	var boxes int64
	db.Model(&Mailbox{}).Where("owner_id = ?", 1).Count(&boxes)
	if boxes != 0 {
		t.Errorf("org 1 mailboxes after purge = %d, want 0", boxes)
	}
	var keep int64
	db.Model(&Mailbox{}).Where("owner_id = ?", 2).Count(&keep)
	if keep != 1 {
		t.Errorf("org 2 mailbox after purge = %d, want 1", keep)
	}
	var blobs int64
	db.Model(&MailRawBlob{}).Count(&blobs)
	if blobs != 0 {
		t.Errorf("blobs after purge = %d, want 0", blobs)
	}
}

func TestExportDataShape(t *testing.T) {
	p, _ := newBouncePlugin(t)
	db := p.db
	mb := Mailbox{OrgID: 1, Address: "exp@example.com", Enabled: true}
	db.Create(&mb)
	db.Create(&Email{MailboxID: mb.ID, FromAddr: "a@b.c", Subject: "e1", ReceivedAt: time.Now()})
	db.Create(&SMTPSender{OrgID: 1, Name: "relay", Host: "smtp.example.com", Port: 587})
	db.Create(&MailSuppression{OrgID: 1, Address: "blocked@example.com", Reason: "manual"})

	data := p.exportData(1)
	for _, k := range []string{"mailboxes", "emails", "smtpSenders", "suppressions"} {
		if _, ok := data[k]; !ok {
			t.Errorf("exportData missing key %q", k)
		}
	}
	if len(data["mailboxes"].([]Mailbox)) != 1 || len(data["emails"].([]Email)) != 1 {
		t.Errorf("export counts wrong: %+v", data)
	}
}

func TestGetStorageProviderBackends(t *testing.T) {
	p, _ := newBouncePlugin(t)

	// Default (no global setting) -> database provider.
	prov, err := p.getStorageProvider()
	if err != nil {
		t.Fatalf("default backend: %v", err)
	}
	if _, isDB := prov.(*DBStorageProvider); !isDB {
		t.Errorf("default backend type = %T, want *DBStorageProvider", prov)
	}

	// Explicit "database" / "db" backends stay on the database provider.
	p.getGlobalSetting = func(key string) string {
		if key == "mail_storage_backend" {
			return "db"
		}
		return ""
	}
	if prov, err := p.getStorageProvider(); err != nil || prov == nil {
		t.Errorf("explicit db backend: %v", err)
	}

	// A configured non-database backend with no registered provider -> Pro error.
	p.getGlobalSetting = func(key string) string {
		if key == "mail_storage_backend" {
			return "s3"
		}
		return ""
	}
	if _, err := p.getStorageProvider(); err == nil {
		t.Error("s3 backend with no provider must error with the Pro message")
	}

	// A registered non-DB provider is returned as-is under the s3 backend.
	custom := &fakeStorageProvider{}
	p.ctx = &plugin.Context{Lookup: func(name string) (any, bool) {
		if name == plugin.ServiceMailStorageProvider {
			return custom, true
		}
		return nil, false
	}}
	got, err := p.getStorageProvider()
	if err != nil || got != custom {
		t.Errorf("s3 backend with custom provider: got %T err=%v", got, err)
	}

	// A registered provider still applies on the default backend.
	p.getGlobalSetting = nil
	got2, err := p.getStorageProvider()
	if err != nil || got2 != custom {
		t.Errorf("default backend with provider: got %T err=%v", got2, err)
	}

	// A registered DB provider under the s3 backend is treated as absent.
	dbP := NewDBStorageProvider(p.db)
	p.ctx = &plugin.Context{Lookup: func(name string) (any, bool) {
		if name == plugin.ServiceMailStorageProvider {
			return dbP, true
		}
		return nil, false
	}}
	p.getGlobalSetting = func(key string) string {
		if key == "mail_storage_backend" {
			return "s3"
		}
		return ""
	}
	if _, err := p.getStorageProvider(); err == nil {
		t.Error("registered DBProvider under s3 backend must still error")
	}
}

type fakeStorageProvider struct {
	data     map[string][]byte
	onDelete func()
}

func (*fakeStorageProvider) Put(context.Context, string, []byte) error { return nil }
func (f *fakeStorageProvider) Get(_ context.Context, key string) ([]byte, error) {
	if f.data != nil {
		if b, ok := f.data[key]; ok {
			return b, nil
		}
	}
	return nil, plugin.ErrStorageNotFound
}
func (f *fakeStorageProvider) Delete(context.Context, string) error {
	if f.onDelete != nil {
		f.onDelete()
	}
	return nil
}
func (*fakeStorageProvider) Stat(context.Context, string) (int64, error) {
	return 0, plugin.ErrStorageNotFound
}

func TestSendMailErrorPaths(t *testing.T) {
	p, _ := newBouncePlugin(t)
	db := p.db
	p.decrypt = func(encoded string) ([]byte, error) { return []byte(encoded), nil }

	// Suppressed recipient refused before any sender lookup.
	db.Create(&MailSuppression{OrgID: 1, Address: "blocked@example.com", Reason: "manual"})
	if err := p.sendMail(1, "blocked@example.com", "s", "b", "b"); err == nil {
		t.Error("sendMail to a suppressed address must fail")
	}

	// No sender configured for the org.
	if err := p.sendMail(1, "ok@example.com", "s", "b", "b"); err == nil {
		t.Error("sendMail with no sender for org must fail")
	}

	// Decrypt failure is surfaced.
	s := SMTPSender{OrgID: 1, Name: "relay", Host: "smtp.example.com", Port: 587, User: "u", Pass: "enc"}
	db.Create(&s)
	p.decrypt = func(encoded string) ([]byte, error) { return nil, errors.New("cannot decrypt") }
	if err := p.sendMail(1, "ok@example.com", "s", "b", "b"); err == nil {
		t.Error("sendMail must fail when the stored password cannot be decrypted")
	}
}

func TestSendSystemMailFailurePublishesEvent(t *testing.T) {
	p, _ := newBouncePlugin(t)
	db := p.db
	p.decrypt = func(encoded string) ([]byte, error) { return []byte(encoded), nil }

	var failed *int
	p.publishEvent = func(orgID uint, event string, data any) {
		if event == "email.send_failed" {
			n := 1
			failed = &n
		}
	}

	// A sender pointing at an unreachable port: the send fails and the event fires.
	db.Create(&SMTPSender{OrgID: 1, Name: "dead", Host: "127.0.0.1", Port: 1, User: "u", Pass: "p", FromEmail: "noreply@example.com"})
	if err := p.sendSystemMail("to@example.com", "subj", "", "body"); err == nil {
		t.Fatal("sendSystemMail to an unreachable relay must fail")
	}
	if failed == nil {
		t.Error("email.send_failed event not published on send failure")
	}
}

func TestSendSystemMailSuccessRecordsUsage(t *testing.T) {
	p, _ := newBouncePlugin(t)
	db := p.db
	p.decrypt = func(encoded string) ([]byte, error) { return []byte(encoded), nil }

	host, port, _ := captureSMTP(t)
	db.Create(&SMTPSender{OrgID: 3, Name: "relay", Host: host, Port: port, User: "u", Pass: "p", FromEmail: "noreply@example.com"})

	usage := 0
	p.recordUsage = func(orgID uint, metric string, n int64) {
		if metric == usagemetric.MailOut {
			usage++
		}
	}
	if err := p.sendSystemMail("to@example.com", "subj", "", "body"); err != nil {
		t.Fatalf("sendSystemMail: %v", err)
	}
	if usage != 1 {
		t.Errorf("recordUsage calls = %d, want 1", usage)
	}
}

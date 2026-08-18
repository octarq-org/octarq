package mail

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func TestIsAWSSNSURL(t *testing.T) {
	valid := []string{
		"https://sns.us-east-1.amazonaws.com/confirm",
		"https://sns.ap-southeast-2.amazonaws.com/x?t=1",
	}
	for _, u := range valid {
		if !isAWSSNSURL(u) {
			t.Errorf("isAWSSNSURL(%q) = false, want true", u)
		}
	}
	invalid := []string{
		"http://sns.us-east-1.amazonaws.com/x", // http, not https
		"https://evil.com/x",                   // not AWS
		"https://sns.example.com/x",            // wrong suffix
		"https://sns.us-east-1.aws-not.com/x",
		"://bad",
		"",
	}
	for _, u := range invalid {
		if isAWSSNSURL(u) {
			t.Errorf("isAWSSNSURL(%q) = true, want false", u)
		}
	}
}

func TestExtractBounceEvents(t *testing.T) {
	t.Parallel()

	// SES Bounce with two recipients.
	ses := `{"notificationType":"Bounce","bounce":{"bounceType":"Permanent","bounceSubType":"General",
		"bouncedRecipients":[{"emailAddress":"a@example.com"},{"emailAddress":"b@example.com"}]}}`
	got := extractBounceEvents([]byte(ses))
	if len(got) != 2 || got[0].Event != "bounce" || got[0].BounceType != "Permanent" || got[0].Email != "a@example.com" {
		t.Errorf("SES bounce parse: %+v", got)
	}
	if !strings.Contains(got[0].Details, "Bounce Type: Permanent, SubType: General") {
		t.Errorf("SES bounce details: %q", got[0].Details)
	}

	// SES complaint.
	sesComplaint := `{"notificationType":"Complaint","complaint":{"complaintFeedbackType":"abuse",
		"complainedRecipients":[{"emailAddress":"c@example.com"}]}}`
	got = extractBounceEvents([]byte(sesComplaint))
	if len(got) != 1 || got[0].Event != "complaint" || got[0].Email != "c@example.com" {
		t.Errorf("SES complaint parse: %+v", got)
	}

	// Mailgun failure with permanent severity.
	mailgun := `{"event-data":{"event":"failed","severity":"permanent","recipient":"d@example.com",
		"delivery-status":{"description":"smtp; 550 5.1.1 doesn't exist"}}}`
	got = extractBounceEvents([]byte(mailgun))
	if len(got) != 1 || got[0].Event != "bounce" || got[0].BounceType != "Permanent" || got[0].Email != "d@example.com" {
		t.Errorf("Mailgun failure parse: %+v", got)
	}
	if !strings.Contains(got[0].Details, "550") {
		t.Errorf("Mailgun details: %q", got[0].Details)
	}

	// Mailgun transient severity maps to Transient.
	mailgunTmp := `{"event-data":{"event":"failed","severity":"temporary","recipient":"e@example.com"}}`
	got = extractBounceEvents([]byte(mailgunTmp))
	if len(got) != 1 || got[0].BounceType != "Transient" {
		t.Errorf("Mailgun transient parse: %+v", got)
	}

	// Mailgun complaint.
	mailgunComplaint := `{"event-data":{"event":"complained","recipient":"f@example.com"}}`
	got = extractBounceEvents([]byte(mailgunComplaint))
	if len(got) != 1 || got[0].Event != "complaint" {
		t.Errorf("Mailgun complaint parse: %+v", got)
	}

	// Generic/SendGrid shape with alternate keys.
	generic := `{"email":"g@example.com","event":"bounce","type":"permanent","reason":"550 unknown"}`
	got = extractBounceEvents([]byte(generic))
	if len(got) != 1 || got[0].Event != "bounce" || got[0].BounceType != "Permanent" || got[0].Email != "g@example.com" {
		t.Errorf("generic parse: %+v", got)
	}

	// SendGrid spamreport counts as a complaint.
	spam := `{"rcpt":"h@example.com","eventType":"spamreport"}`
	got = extractBounceEvents([]byte(spam))
	if len(got) != 1 || got[0].Event != "complaint" || got[0].Email != "h@example.com" {
		t.Errorf("spamreport parse: %+v", got)
	}

	// A list of events is handled element-wise.
	list := `[{"email":"i@example.com","event":"bounce"},{"email":"j@example.com","event":"complaint"}]`
	got = extractBounceEvents([]byte(list))
	if len(got) != 2 {
		t.Errorf("list parse: %+v", got)
	}

	// Unknown / malformed payloads yield nothing.
	for _, bad := range []string{`{"unrelated":true}`, `<html>not json</html>`, `[]`} {
		if got := extractBounceEvents([]byte(bad)); len(got) != 0 {
			t.Errorf("payload %s parsed into %+v; want none", bad, got)
		}
	}
}

// newBouncePlugin builds a mail plugin over a fresh DB with a single org +
// token, so webhook tests can drive the public bounce endpoint without tripping
// over rows a previous rerun left behind.
func newBouncePlugin(t *testing.T) (*Plugin, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	p := New()
	if err := db.AutoMigrate(append(append(models.AllModels(), p.Models()...), &dns.Domain{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&models.Org{})
	db.Where("1 = 1").Delete(&Mailbox{})
	db.Where("1 = 1").Delete(&Email{})
	db.Where("1 = 1").Delete(&SMTPSender{})
	db.Where("1 = 1").Delete(&MailSuppression{})
	db.Where("1 = 1").Delete(&MailRawBlob{})
	db.Where("1 = 1").Delete(&models.AuditLog{})
	db.Where("1 = 1").Delete(&dns.Domain{})
	p.Mount(nil, pluginCtxForTest(db))
	return p, db
}

func bounceInput(orgSlug, token, body string) *EmailBounceWebhookInput {
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/"+orgSlug+"/email/bounce/"+token, bytes.NewBufferString(body))
	return &EmailBounceWebhookInput{
		Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
		OrgSlug: orgSlug,
		Token:   token,
	}
}

func TestEmailBounceWebhookEdges(t *testing.T) {
	t.Parallel()
	p, db := newBouncePlugin(t)
	ctx := context.Background()

	db.Create(&models.Org{ID: 1, Slug: "acme", InboundToken: "tok"})
	db.Create(&Mailbox{OrgID: 1, Address: "user@acme.example", Enabled: true})
	// Extra org to prove tenant scoping of suppression writes.
	db.Create(&models.Org{ID: 2, Slug: "other", InboundToken: "tok2"})

	// Unknown org -> 404.
	if _, err := p.emailBounceWebhook(ctx, bounceInput("nope", "tok", `{}`)); err == nil {
		t.Error("unknown org must 404")
	}
	// Wrong token -> 401.
	if _, err := p.emailBounceWebhook(ctx, bounceInput("acme", "wrong", `{}`)); err == nil {
		t.Error("bad token must 401")
	}
	// SubscriptionConfirmation with a non-AWS SubscribeURL is refused (SSRF).
	if _, err := p.emailBounceWebhook(ctx, bounceInput("acme", "tok", `{"Type":"SubscriptionConfirmation","SubscribeURL":"http://internal.example/x"}`)); err == nil {
		t.Error("non-AWS SubscribeURL must be refused")
	}
	// A bounce for a mailbox that does not exist is skipped, still ok.
	out, err := p.emailBounceWebhook(ctx, bounceInput("acme", "tok", `{"recipient":"ghost@nope.example","event":"bounce"}`))
	if err != nil {
		t.Fatalf("webhook with unmatched mailbox: %v", err)
	}
	if out.Body["processed"] != 0 {
		t.Errorf("processed = %v, want 0 for unmatched mailbox", out.Body["processed"])
	}

	// A suppression-relevant bounce lands on the RIGHT org's list only.
	out2, err := p.emailBounceWebhook(ctx, bounceInput("acme", "tok", `{"notificationType":"Bounce","bounce":{"bounceType":"Permanent","bouncedRecipients":[{"emailAddress":"user@acme.example"}]}}`))
	if err != nil {
		t.Fatalf("hard bounce: %v", err)
	}
	if out2.Body["processed"] != 1 {
		t.Errorf("processed = %v, want 1", out2.Body["processed"])
	}
	var inAcme int64
	db.Model(&MailSuppression{}).Where("owner_id = ? AND address = ?", 1, "user@acme.example").Count(&inAcme)
	if inAcme != 1 {
		t.Errorf("suppression for org 1 not written (count %d)", inAcme)
	}
	var inOther int64
	db.Model(&MailSuppression{}).Where("owner_id = ?", 2).Count(&inOther)
	if inOther != 0 {
		t.Errorf("org 2 received rows it must not have: %d", inOther)
	}
}

func TestReporterIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if got := reporterIP(r); got != "1.2.3.4" {
		t.Errorf("XFF with hops: got %q, want 1.2.3.4", got)
	}
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("X-Forwarded-For", "9.9.9.9")
	if got := reporterIP(r2); got != "9.9.9.9" {
		t.Errorf("single XFF: got %q, want 9.9.9.9", got)
	}
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	r3.RemoteAddr = "10.0.0.5:1234"
	if got := reporterIP(r3); got != "10.0.0.5:1234" {
		t.Errorf("RemoteAddr fallback: got %q", got)
	}
}

func TestIsReservedMailbox(t *testing.T) {
	p := &Plugin{}
	reserved := []string{"admin@x.com", "POSTMASTER@x.com", "webmaster@x.com", "hostmaster@x.com", "administrator@x.com"}
	for _, a := range reserved {
		if !p.isReservedMailbox(1, a) {
			t.Errorf("%q must be reserved", a)
		}
	}
	if p.isReservedMailbox(1, "sales@x.com") {
		t.Error("sales@x.com must not be reserved")
	}
	if p.isReservedMailbox(1, "admin") {
		t.Error("address without @ must not be treated as reserved")
	}
}

func TestMailHostDisabledEdge(t *testing.T) {
	p := &Plugin{}
	if p.mailHostDisabled(0, "any.example") {
		t.Error("mailHostDisabled with org 0 must be false")
	}
}

func TestMailAddressDomainNotAnotherTenants(t *testing.T) {
	p := &Plugin{}
	if p.mailAddressDomainNotAnotherTenants(0, "a@b.com") {
		t.Error("org 0 must be refused")
	}
	if p.mailAddressDomainNotAnotherTenants(1, "no-at") {
		t.Error("address without @ must be refused")
	}
	if p.mailAddressDomainNotAnotherTenants(1, "user@") {
		t.Error("empty domain must be refused")
	}

	_, db := newBouncePlugin(t)
	p.db = db
	// victim.example is another tenant's mail host.
	db.Create(&dns.Domain{OrgID: 2, Name: "victim.example", ForMail: true,
		MailHosts: models.HostList{{Host: "victim.example", Enabled: true}}})
	// own.example is this tenant's own.
	db.Create(&dns.Domain{OrgID: 1, Name: "own.example", ForMail: true,
		MailHosts: models.HostList{{Host: "own.example", Enabled: true}}})
	if p.mailAddressDomainNotAnotherTenants(1, "user@victim.example") {
		t.Error("another tenant's mail host must be refused")
	}
	if !p.mailAddressDomainNotAnotherTenants(1, "user@own.example") {
		t.Error("own domain must be allowed")
	}
	if !p.mailAddressDomainNotAnotherTenants(1, "user@unregistered.example") {
		t.Error("unregistered domain must be allowed")
	}
}

func TestResolveMailboxCatchAllCreates(t *testing.T) {
	t.Parallel()
	p, db := newBouncePlugin(t)

	p.getWorkspaceSetting = func(orgID uint, key string) string {
		return "true"
	}
	// Org 1 owns mail host mail.hosted.example (explicit) and apex catchall.example (bare).
	db.Create(&dns.Domain{OrgID: 1, Name: "catchall.example", ForMail: true})
	db.Create(&dns.Domain{OrgID: 1, Name: "hosted.example", ForMail: true,
		MailHosts: models.HostList{{Host: "mail.hosted.example", Enabled: true}},
	})

	// Catch-all auto-creates on an owned mail host.
	mb, ok := p.resolveMailbox(1, "team@catchall.example")
	if !ok {
		t.Fatal("catch-all should create a mailbox for an owned host")
	}
	if mb.Note != "auto (catch-all)" {
		t.Errorf("auto-created mailbox note = %q", mb.Note)
	}
	if _, ok := p.resolveMailbox(1, "team@catchall.example"); !ok {
		t.Error("the auto-created mailbox must resolve on the second lookup")
	}

	// Reserved local-part is never auto-created.
	if _, ok := p.resolveMailbox(1, "postmaster@catchall.example"); ok {
		t.Error("reserved local-part must not be auto-created")
	}

	// A host the org does not own is not auto-created.
	if _, ok := p.resolveMailbox(1, "x@notours.example"); ok {
		t.Error("unowned host must not be auto-created")
	}

	// Empty address never resolves.
	if _, ok := p.resolveMailbox(1, ""); ok {
		t.Error("empty address must not resolve")
	}
}

func pluginCtxForTest(db *gorm.DB) *plugin.Context {
	return &plugin.Context{
		DB:          db,
		OrgID:       func(*http.Request) uint { return 1 },
		RequireRole: func(*http.Request, string) bool { return true },
	}
}

// wipeMailTables clears rows a previous run of the same test (SQLite in-memory
// DBs with cache=shared persist across -count=2 reruns) may have left behind.
func wipeMailTables(t *testing.T, p *Plugin) {
	t.Helper()
	p.db.Where("1 = 1").Delete(&Mailbox{})
	p.db.Where("1 = 1").Delete(&Email{})
	p.db.Where("1 = 1").Delete(&SMTPSender{})
	p.db.Where("1 = 1").Delete(&MailSuppression{})
	p.db.Where("1 = 1").Delete(&MailRawBlob{})
	p.db.Where("1 = 1").Delete(&models.AuditLog{})
	p.db.Where("1 = 1").Delete(&models.Org{})
	p.db.Where("1 = 1").Delete(&dns.Domain{})
}

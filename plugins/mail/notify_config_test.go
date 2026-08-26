package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// TestNotifyReceivesStoredConfigVerbatim is the regression test for a silent
// delivery failure.
//
// Notification channel configs are encrypted at rest. The mail plugin used to
// json.Unmarshal the stored value into a map and hand that to the core, which
// re-marshalled it. Once configs were encrypted the unmarshal failed, the error
// was discarded (`json.Unmarshal(...)` with no error check), the map stayed nil,
// and every inbound-email and reputation notification went out with an empty
// config — silently, forever.
//
// The contract now is: the plugin passes the stored value through untouched and
// the core's dispatch owns the decrypt. This test pins that by asserting the
// exact bytes the plugin forwards.
func TestNotifyReceivesStoredConfigVerbatim(t *testing.T) {
	const stored = "v1:AAAABBBBCCCC==" // shaped like ciphertext: not valid JSON

	var gotType, gotConfig, gotText string
	calls := 0

	p := New()
	p.Mount(nil, &plugin.Context{
		Notify: func(_ context.Context, kind, cfgJSON, message string) error {
			calls++
			gotType, gotConfig, gotText = kind, cfgJSON, message
			return nil
		},
	})

	if p.notify == nil {
		t.Fatal("Mount did not wire the notify hook")
		return
	}
	if err := p.notify(context.Background(), "telegram", stored, "hello"); err != nil {
		t.Fatalf("notify: %v", err)
	}

	if calls != 1 {
		t.Fatalf("notify called %d times, want 1", calls)
	}
	if gotConfig != stored {
		t.Errorf("config was altered in transit:\n got  %q\n want %q\nA plugin that parses the stored config cannot handle an encrypted one.", gotConfig, stored)
	}
	if gotType != "telegram" || gotText != "hello" {
		t.Errorf("type/text mangled: type=%q text=%q", gotType, gotText)
	}
}

// TestEncryptedConfigReachesProvider drives the whole path the webhook uses —
// stored ciphertext → plugin → core dispatch → registered provider — and asserts
// the provider is handed decrypted, parseable JSON.
func TestEncryptedConfigReachesProvider(t *testing.T) {
	plain := `{"botToken":"secret-token","chatId":"42"}`
	const sealed = "sealed:" + `{"botToken":"secret-token","chatId":"42"}`

	// Stand in for the real cipher: the core injects the decryptor the same way
	// (app wiring calls notify.SetConfigDecryptor).
	notify.SetConfigDecryptor(func(stored string) (string, bool) {
		if len(stored) > 7 && stored[:7] == "sealed:" {
			return stored[7:], true
		}
		return "", false
	})
	t.Cleanup(func() { notify.SetConfigDecryptor(nil) })

	var delivered string
	notify.Register("test-provider", func(_ context.Context, cfgJSON, _ string) error {
		delivered = cfgJSON
		return nil
	})

	p := New()
	p.Mount(nil, &plugin.Context{Notify: notify.Send})

	if err := p.notify(context.Background(), "test-provider", sealed, "body"); err != nil {
		t.Fatalf("notify: %v", err)
	}

	if delivered != plain {
		t.Fatalf("provider got %q, want the decrypted config %q", delivered, plain)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(delivered), &cfg); err != nil {
		t.Fatalf("provider got something that is not JSON (%q): %v", delivered, err)
	}
	if cfg["botToken"] != "secret-token" {
		t.Errorf("botToken = %v, want secret-token", cfg["botToken"])
	}
}

// TestInboundWebhookForwardsStoredConfig covers the actual call site. The two
// tests above pin the wiring and the dispatch; this one drives a real inbound
// delivery so that passing anything other than the stored config — the original
// bug — is caught where it happened.
func TestInboundWebhookForwardsStoredConfig(t *testing.T) {
	const sealedConfig = "sealed:{\"botToken\":\"t\",\"chatId\":\"1\"}"

	db, err := gorm.Open(sqlite.Open("file:mailnotify?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Mailbox{}, &Email{}, &SMTPSender{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const orgID uint = 5
	org := models.Org{Slug: "acme", InboundToken: "tok-inbound"}
	org.ID = orgID
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if err := db.Create(&Mailbox{OrgID: orgID, Address: "hi@acme.test", Enabled: true}).Error; err != nil {
		t.Fatalf("seed mailbox: %v", err)
	}
	if err := db.Create(&models.NotificationChannel{
		OrgID: orgID, Name: "tg", Type: "telegram", Config: sealedConfig, Enabled: true,
	}).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	got := make(chan string, 4)
	p := New()
	p.Mount(nil, &plugin.Context{
		DB: db,
		Notify: func(_ context.Context, _, cfgJSON, _ string) error {
			got <- cfgJSON
			return nil
		},
	})

	raw := "From: a@b.test\r\nTo: hi@acme.test\r\nSubject: hello\r\n\r\nbody\r\n"
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/tok-inbound", strings.NewReader(raw))
	req.Header.Set("X-Octarq-To", "hi@acme.test")
	rec := httptest.NewRecorder()

	if _, err := p.inbound(context.Background(), &InboundInput{
		Ctx:     humago.NewContext(nil, req, rec),
		OrgSlug: "acme",
		Token:   "tok-inbound",
	}); err != nil {
		t.Fatalf("inbound: %v", err)
	}

	select {
	case cfg := <-got:
		if cfg != sealedConfig {
			t.Errorf("notification sent with %q, want the stored config %q — the webhook must not transform it", cfg, sealedConfig)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no notification dispatched for the inbound email")
	}
}

// Compile-time guard: notify must take the config as an opaque string. Typing it
// as a parsed map is what made this plugin decode ciphertext in the first place,
// so a change back to map[string]any should fail the build, not a test run.
//
// Written as a call rather than `var _ T = p.notify`, which staticcheck's QF1011
// rejects for a "redundant" type — the type is the entire point here.
func assertNotifyHookShape(func(ctx context.Context, kind, cfgJSON, message string) error) {}

var _ = func(p *Plugin) { assertNotifyHookShape(p.notify) }

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin/safehttp"
)

// wireEventbusForTest mirrors app.Run's eventbus wiring (db + secret
// decryptor) against the harness cipher, and opts loopback receivers in for
// the httptest endpoint. Cleanup restores the globals so one test's decryptor
// can never leak another test's secrets.
// Route-registration guards: these hit the composed mux, so deleting the
// huma.Register line turns them into 404s — direct handler-call tests cannot
// see that regression.
func TestProviderTestRoutesRegistered(t *testing.T) {
	srv, _ := newTestHandler(t)

	rec := do(srv, "POST", "/api/webhooks/1/test", nil, "")
	if rec.Code == http.StatusNotFound {
		t.Error("/api/webhooks/{id}/test is not registered")
	}
	// Unauthenticated must be rejected before any lookup.
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated webhook test: got %d, want 401", rec.Code)
	}

	rec = do(srv, "POST", "/api/smtp-senders/1/test", nil, "")
	if rec.Code == http.StatusNotFound {
		t.Error("/api/smtp-senders/{id}/test is not registered")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated smtp sender test: got %d, want 401", rec.Code)
	}
}

func wireEventbusForTest(t *testing.T, decrypt func(string) (string, bool)) {
	t.Helper()
	eventbus.SetSecretDecryptor(decrypt)
	safehttp.SetAllowPrivateWebhooks(true)
	t.Cleanup(func() {
		eventbus.SetSecretDecryptor(nil)
		safehttp.SetAllowPrivateWebhooks(false)
	})
}

func TestWebhookTestEndpoint_AuthzAndScoping(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	memberUser := models.User{Email: "memberwht@example.com"}
	db.Create(&memberUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: memberUser.ID, Role: "member"})
	memberCookies := sessionCookies(t, memberUser.ID, 1)

	rec := do(srv, "POST", "/api/webhooks/99999/test", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated webhook test: got %d, want 401", rec.Code)
	}

	rec = do(srv, "POST", "/api/webhooks/99999/test", memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member webhook test: got %d, want 403", rec.Code)
	}

	rec = do(srv, "POST", "/api/webhooks/99999/test", adminCookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown webhook id: got %d, want 404", rec.Code)
	}

	// A foreign org's webhook id must read as not-found, not as a probe oracle.
	hook := models.Webhook{OrgID: 2, Name: "other-org", URL: "https://example.com/wh", Secret: "x", Events: "*", Enabled: true}
	db.Create(&hook)
	rec = do(srv, "POST", fmt.Sprintf("/api/webhooks/%d/test", hook.ID), adminCookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-org webhook test: got %d, want 404", rec.Code)
	}

	// Disabled webhooks refuse the test instead of pretending to verify.
	disabled := models.Webhook{OrgID: 1, Name: "disabled", URL: "https://example.com/wh", Secret: "x", Events: "*", Enabled: false}
	db.Create(&disabled)
	// gorm skips zero-value bools on Create, so pin enabled=false explicitly.
	db.Model(&models.Webhook{}).Where("id = ?", disabled.ID).Update("enabled", false)
	rec = do(srv, "POST", fmt.Sprintf("/api/webhooks/%d/test", disabled.ID), adminCookies, "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "disabled") {
		t.Fatalf("disabled webhook test: got %d (%s), want 400 mentioning disabled", rec.Code, rec.Body.String())
	}
}

func TestWebhookTestEndpoint_DeliversSignedEvent(t *testing.T) {
	var body []byte
	var sigV2 string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 0)
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			b = append(b, buf[:n]...)
			if err != nil {
				break
			}
		}
		body = b
		sigV2 = r.Header.Get("X-Octarq-Signature-V2")
		w.WriteHeader(200)
	}))
	defer receiver.Close()

	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "test-webhook-secret"}
	h, srv, _ := newTestHandlerRawCfg(t, cfg)
	// Sign with the handler's own enveloped cipher — a fresh crypto.New over
	// the same key would not unwrap values sealed through apiEnvStore.
	wireEventbusForTest(t, func(stored string) (string, bool) {
		pt, err := h.cipher.Decrypt(stored)
		if err != nil {
			return "", false
		}
		return string(pt), true
	})
	adminCookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/webhooks", adminCookies, fmt.Sprintf(`{"name":"probe","url":%q}`, receiver.URL))
	if rec.Code >= 300 {
		t.Fatalf("create webhook: got %d (%s)", rec.Code, rec.Body.String())
	}
	var created models.Webhook
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created webhook: %v", err)
	}

	rec = do(srv, "POST", fmt.Sprintf("/api/webhooks/%d/test", created.ID), adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook test against live receiver: got %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || !out.OK {
		t.Fatalf("bad response body %q (err=%v)", rec.Body.String(), err)
	}

	if len(body) == 0 {
		t.Fatal("receiver saw no delivery")
	}
	var payload struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Event != "webhook.test" {
		t.Fatalf("delivery is not a webhook.test event: %q (err=%v)", body, err)
	}
	if sigV2 == "" {
		t.Error("delivery carried no v2 signature")
	}

	// A receiver that answers non-2xx must surface as a 400 with its reason.
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer failing.Close()
	rec = do(srv, "POST", "/api/webhooks", adminCookies, fmt.Sprintf(`{"name":"failing","url":%q}`, failing.URL))
	var bad models.Webhook
	json.Unmarshal(rec.Body.Bytes(), &bad)
	rec = do(srv, "POST", fmt.Sprintf("/api/webhooks/%d/test", bad.ID), adminCookies, "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "500") {
		t.Fatalf("failing receiver: got %d (%s), want 400 mentioning HTTP 500", rec.Code, rec.Body.String())
	}
}

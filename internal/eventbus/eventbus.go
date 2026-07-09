package eventbus

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/safehttp"
	"gorm.io/gorm"
)

var (
	db *gorm.DB
	// SSRF-hardened: webhook URLs are tenant-supplied, so delivery must not reach
	// internal services or cloud metadata (relaxable for trusted self-hosted
	// receivers via OCTARQ_ALLOW_PRIVATE_WEBHOOKS).
	httpClient    = safehttp.NewWebhookClient(10 * time.Second)
	decryptSecret func(string) (string, bool)
)

// Init initializes the eventbus with the shared GORM database connection.
func Init(gdb *gorm.DB) {
	db = gdb
}

// SetSecretDecryptor registers how a stored (encrypted) webhook secret is
// unwrapped before it is used to HMAC-sign the payload.
func SetSecretDecryptor(fn func(string) (string, bool)) {
	decryptSecret = fn
}

// signingSecret resolves the plaintext HMAC secret for a stored value, falling
// back to the raw value for legacy plaintext rows or when no decryptor is set.
func signingSecret(stored string) string {
	if decryptSecret != nil {
		if pt, ok := decryptSecret(stored); ok {
			return pt
		}
	}
	return stored
}

// EventPayload defines the JSON structure sent to webhook endpoints.
type EventPayload struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	OrgID     uint      `json:"orgId"`
	Data      any       `json:"data"`
}

// Publish dispatches an event asynchronously to all subscribed webhooks.
func Publish(orgID uint, event string, data any) {
	if db == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var hooks []models.Webhook
		err := db.WithContext(ctx).Where("owner_id = ? AND enabled = ?", orgID, true).Find(&hooks).Error
		if err != nil {
			log.Printf("eventbus: failed to query webhooks: %v", err)
			return
		}

		if len(hooks) == 0 {
			return
		}

		payload := EventPayload{
			Event:     event,
			Timestamp: time.Now(),
			OrgID:     orgID,
			Data:      data,
		}

		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			log.Printf("eventbus: failed to marshal payload: %v", err)
			return
		}

		for _, hook := range hooks {
			if !isSubscribed(hook.Events, event) {
				continue
			}
			go func(h models.Webhook) {
				deliverCtx, cancelDeliver := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancelDeliver()
				deliver(deliverCtx, h.URL, h.Secret, bodyBytes)
			}(hook)
		}
	}()
}

// isSubscribed checks if the comma-separated subscriptions string matches the event.
func isSubscribed(subs, event string) bool {
	if subs == "*" {
		return true
	}
	parts := strings.Split(subs, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "*" || p == event {
			return true
		}
	}
	return false
}

// deliver posts the payload to the URL with the HMAC signature header.
func deliver(ctx context.Context, url, secret string, body []byte) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	if err := safehttp.ValidateScheme(req.URL.Scheme); err != nil {
		log.Printf("eventbus: refusing webhook delivery to %s: %v", url, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "octarq-webhook-dispatcher/1.0")

	// Calculate HMAC-SHA256 signature over the plaintext signing secret.
	mac := hmac.New(sha256.New, []byte(signingSecret(secret)))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	req.Header.Set("X-Octarq-Signature", "sha256="+sig)

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("eventbus: deliver to %s failed: %v", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("eventbus: deliver to %s returned HTTP status %d", url, resp.StatusCode)
	}
}

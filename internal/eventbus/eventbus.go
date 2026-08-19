package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

// signingSecret resolves the plaintext HMAC secret for a stored value. A stored
// secret that cannot be decrypted, or a build with no decryptor registered, is
// an error rather than a silent passthrough of whatever was stored.
func signingSecret(stored string) (string, error) {
	if decryptSecret == nil {
		return "", errors.New("no secret decryptor registered")
	}
	pt, ok := decryptSecret(stored)
	if !ok {
		return "", errors.New("stored webhook secret could not be decrypted")
	}
	return pt, nil
}

// EventPayload defines the JSON structure sent to webhook endpoints.
type EventPayload struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	OrgID     uint      `json:"orgId"`
	Data      any       `json:"data"`
}

// Publish dispatches an event asynchronously to all subscribed webhooks.
//
// Each (event, hook) pair becomes one delivery with its own id, retry budget
// and persisted log row. Fan-out is bounded by deliverySem: a tenant with a
// long hook list queues rather than spawning a goroutine per hook.
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

		signedAt := nowFn()
		payload := EventPayload{
			Event:     event,
			Timestamp: signedAt,
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
			d := delivery{
				DeliveryID: newDeliveryID(),
				OrgID:      orgID,
				WebhookID:  hook.ID,
				Event:      event,
				URL:        hook.URL,
				Secret:     hook.Secret,
				Body:       bodyBytes,
				SignedAt:   signedAt,
			}
			deliverySem <- struct{}{}
			go func(d delivery) {
				defer func() { <-deliverySem }()
				// Detached from the query context on purpose: the retry budget
				// outlives the 30s lookup window by design.
				deliverCtx, cancelDeliver := context.WithTimeout(context.Background(), deliveryBudget)
				defer cancelDeliver()
				deliverWithRetry(deliverCtx, d)
			}(d)
		}
	}()
}

// deliveryBudget caps the whole attempt+backoff loop for one delivery, so a
// permanently dead receiver cannot pin a slot in deliverySem forever.
var deliveryBudget = 15 * time.Minute

// webhookSecret loads the stored (still encrypted) signing secret for a hook.
func webhookSecret(ctx context.Context, webhookID uint) (string, error) {
	if db == nil {
		return "", errors.New("eventbus: not initialised")
	}
	var hook models.Webhook
	if err := db.WithContext(ctx).Where("id = ?", webhookID).First(&hook).Error; err != nil {
		return "", fmt.Errorf("eventbus: webhook %d: %w", webhookID, err)
	}
	return hook.Secret, nil
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

package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
	"gorm.io/gorm"
)

var (
	dbMu sync.RWMutex
	db   *gorm.DB
	// SSRF-hardened: webhook URLs are tenant-supplied, so delivery must not reach
	// internal services or cloud metadata (relaxable for trusted self-hosted
	// receivers via OCTARQ_ALLOW_PRIVATE_WEBHOOKS).
	httpClient   = safehttp.NewWebhookClient(10 * time.Second)
	vaultMu      sync.RWMutex
	vaultService plugin.VaultService
)

// getDB returns the current DB handle under a read lock.
func getDB() *gorm.DB {
	dbMu.RLock()
	defer dbMu.RUnlock()
	return db
}

// Init initializes the eventbus with the shared GORM database connection.
func Init(gdb *gorm.DB) {
	dbMu.Lock()
	db = gdb
	dbMu.Unlock()
}

// SetVaultService registers the VaultService used to resolve webhook signing
// secrets before HMAC-signing deliveries.
func SetVaultService(vs plugin.VaultService) {
	vaultMu.Lock()
	vaultService = vs
	vaultMu.Unlock()
}

func getVaultService() plugin.VaultService {
	vaultMu.RLock()
	defer vaultMu.RUnlock()
	return vaultService
}

// signingSecret resolves the plaintext HMAC secret for a webhook's vault item.
// A missing vault service or unresolvable vault item is a hard error rather
// than signing with an empty or placeholder secret.
func signingSecret(ctx context.Context, orgID, vaultItemID uint) (string, error) {
	if vaultItemID == 0 {
		return "", errors.New("webhook has no vault secret")
	}
	vs := getVaultService()
	if vs == nil {
		return "", errors.New("no vault service registered")
	}
	sec, err := vs.GetSecret(ctx, orgID, vaultItemID)
	if err != nil {
		return "", fmt.Errorf("stored webhook secret could not be resolved: %w", err)
	}
	s, _ := sec.Payload["signingSecret"].(string)
	if s == "" {
		return "", errors.New("stored webhook secret is empty")
	}
	return s, nil
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
	// Mirror to the in-process event spine so in-process subscribers receive
	// the event even when no webhooks are registered. This is the only allowed
	// touch to this function (spec §3.9, additive-only).
	if raw, err := json.Marshal(data); err == nil {
		PublishEnvelope(Envelope{
			OrgID:   orgID,
			Key:     event,
			Payload: json.RawMessage(raw),
		})
	}
	if getDB() == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		gdb := getDB()
		if gdb == nil {
			return
		}
		var hooks []models.Webhook
		err := gdb.WithContext(ctx).Where("owner_id = ? AND enabled = ?", orgID, true).Find(&hooks).Error
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
				DeliveryID:  newDeliveryID(),
				OrgID:       orgID,
				WebhookID:   hook.ID,
				Event:       event,
				URL:         hook.URL,
				VaultItemID: hook.VaultItemID,
				Body:        bodyBytes,
				SignedAt:    signedAt,
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

// webhookVaultItemID loads the VaultItemID for a hook.
func webhookVaultItemID(ctx context.Context, orgID, webhookID uint) (uint, error) {
	gdb := getDB()
	if gdb == nil {
		return 0, errors.New("eventbus: not initialised")
	}
	var hook models.Webhook
	q := gdb.WithContext(ctx).Where("id = ?", webhookID)
	if orgID > 0 {
		q = q.Where("owner_id = ?", orgID)
	}
	if err := q.First(&hook).Error; err != nil {
		return 0, fmt.Errorf("eventbus: webhook %d: %w", webhookID, err)
	}
	return hook.VaultItemID, nil
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

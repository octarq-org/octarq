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

	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm"
)

var (
	db         *gorm.DB
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

// Init initializes the eventbus with the shared GORM database connection.
func Init(gdb *gorm.DB) {
	db = gdb
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
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "led-webhook-dispatcher/1.0")

	// Calculate HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	req.Header.Set("X-Led-Signature", "sha256="+sig)

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

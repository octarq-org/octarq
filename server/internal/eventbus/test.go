package eventbus

import (
	"context"
	"encoding/json"
	"time"
)

// TestSend delivers one synthetic "webhook.test" event to url and waits for
// the single attempt's result — no retry loop, no fan-out semaphore, no
// persisted delivery-log row. It backs the dashboard's per-webhook "send test"
// button: an operator verifies URL reachability, secret decryption and the
// receiver's signature check in one round trip instead of waiting for a real
// business event.
//
// storedSecret is the webhook's secret AS STORED (encrypted); attempt() resolves
// the plaintext through the registered decryptor exactly like Publish does.
func TestSend(ctx context.Context, orgID, webhookID uint, url, storedSecret string) error {
	signedAt := nowFn()
	body, err := json.Marshal(EventPayload{
		Event:     "webhook.test",
		Timestamp: signedAt,
		OrgID:     orgID,
		Data: map[string]any{
			"message":   "This is a test delivery from octarq. If you can verify its signature, your endpoint is wired up correctly.",
			"webhookId": webhookID,
		},
	})
	if err != nil {
		return err
	}
	d := delivery{
		DeliveryID: newDeliveryID(),
		OrgID:      orgID,
		WebhookID:  webhookID,
		Event:      "webhook.test",
		URL:        url,
		Secret:     storedSecret,
		Body:       body,
		SignedAt:   signedAt,
	}
	attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res := attempt(attemptCtx, d, 1)
	return res.Err
}

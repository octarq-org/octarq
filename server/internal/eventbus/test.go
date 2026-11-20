package eventbus

import (
	"context"
	"encoding/json"
	"time"
)

func TestSend(ctx context.Context, orgID, webhookID uint, url string, vaultItemID uint) error {
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
		DeliveryID:  newDeliveryID(),
		OrgID:       orgID,
		WebhookID:   webhookID,
		Event:       "webhook.test",
		URL:         url,
		VaultItemID: vaultItemID,
		Body:        body,
		SignedAt:    signedAt,
	}
	attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res := attempt(attemptCtx, d, 1)
	return res.Err
}

package eventbus

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/octarq-org/octarq/pkg/telemetry"
	"github.com/octarq-org/octarq/plugin/safehttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// WebhookDelivery is the persisted attempt log for one outbound webhook.
//
// It exists so a failed delivery is visible and replayable instead of being a
// line in the process log that nobody reads. One row per (event, webhook)
// pair; the row is created BEFORE the first HTTP attempt so a crash mid-flight
// leaves evidence, and updated in place as attempts are made.
//
// Body is stored verbatim because it is the signed material: a replay must
// re-send byte-identical content or the receiver's signature check fails.
type WebhookDelivery struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// DeliveryID is the value sent in X-Octarq-Delivery. Receivers dedupe on
	// it, so it is unique and stable across retries AND across replays of the
	// same delivery — a replay is the same delivery, not a new event.
	DeliveryID string `gorm:"uniqueIndex;size:64;not null" json:"deliveryId"`
	OrgID      uint   `gorm:"index;column:owner_id;not null" json:"-"`
	WebhookID  uint   `gorm:"index;not null" json:"webhookId"`
	Event      string `gorm:"size:128;not null" json:"event"`
	URL        string `gorm:"size:1024;not null" json:"url"`
	Body       string `gorm:"type:text" json:"-"`
	// Status is one of pending, delivered, failed. "failed" is the dead-letter
	// state: every attempt was used up and the event will not be retried
	// without an operator-triggered Replay.
	Status       string    `gorm:"size:16;index;not null;default:'pending'" json:"status"`
	Attempts     int       `gorm:"not null;default:0" json:"attempts"`
	ResponseCode int       `gorm:"not null;default:0" json:"responseCode"`
	LastError    string    `gorm:"size:1024" json:"lastError,omitempty"`
	SignedAt     time.Time `json:"signedAt"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Delivery status values.
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

// Models returns the eventbus-owned models for the app's single AutoMigrate
// pass (see app.Run).
func Models() []any { return []any{&WebhookDelivery{}} }

// Retry/fan-out policy. Package vars rather than consts so tests can shrink the
// clock without waiting out real backoff.
var (
	// maxAttempts bounds total HTTP attempts per delivery. Exhausting it moves
	// the row to the dead-letter state (StatusFailed) rather than retrying
	// forever against an endpoint that is gone.
	maxAttempts = 5
	baseBackoff = 1 * time.Second
	maxBackoff  = 5 * time.Minute
	// maxConcurrentDeliveries bounds fan-out. Without it, one event with N
	// subscribed hooks spawns N goroutines, each holding an HTTP connection —
	// a burst of events times a long hook list is an unbounded goroutine and
	// socket leak driven by tenant-controlled data.
	maxConcurrentDeliveries = 16
	deliverySem             = make(chan struct{}, maxConcurrentDeliveries)

	// sleepFn is context-aware so a shutting-down process does not sit in a
	// backoff nap.
	sleepFn = func(ctx context.Context, d time.Duration) {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
		}
	}
	// nowFn and newDeliveryID are seams for deterministic tests.
	nowFn         = time.Now
	newDeliveryID = func() string { return uuid.NewString() }
)

// backoffFor returns the wait before attempt n+1 (n is the number of attempts
// already made): exponential from baseBackoff, capped at maxBackoff, with full
// jitter so a fleet of receivers coming back at once is not stampeded by every
// pending delivery retrying on the same tick.
func backoffFor(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := baseBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= maxBackoff {
			d = maxBackoff
			break
		}
	}
	// Full jitter: uniform in [d/2, d].
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// signature computes the HMAC-SHA256 of material under secret, hex-encoded.
func signature(secret string, material []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(material)
	return hex.EncodeToString(mac.Sum(nil))
}

// v2Material is the signed material for X-Octarq-Signature-V2:
//
//	"<unix-seconds>.<delivery-id>.<body>"
//
// Binding the timestamp and delivery id INTO the signature (rather than only
// sending them as headers) is what makes a replay detectable: an attacker who
// captures a delivery cannot move it forward in time or re-label it without
// invalidating the signature, so a receiver can safely reject anything outside
// its tolerance window (5 minutes is the recommended value) and dedupe on the
// delivery id.
func v2Material(ts time.Time, deliveryID string, body []byte) []byte {
	prefix := strconv.FormatInt(ts.Unix(), 10) + "." + deliveryID + "."
	out := make([]byte, 0, len(prefix)+len(body))
	out = append(out, prefix...)
	return append(out, body...)
}

// delivery is one webhook send: an event body addressed to one endpoint.
type delivery struct {
	DeliveryID string
	OrgID      uint
	WebhookID  uint
	Event      string
	URL        string
	Secret     string // as stored (encrypted); decrypted at signing time
	Body       []byte
	SignedAt   time.Time
}

// attemptResult carries what the delivery log records about one HTTP attempt.
type attemptResult struct {
	StatusCode int
	Err        error
}

// attempt performs exactly one signed POST. A non-2xx response is an error:
// the receiver did not accept the event, so the delivery is not done.
func attempt(ctx context.Context, d delivery, attemptNo int) attemptResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader(d.Body))
	if err != nil {
		return attemptResult{Err: err}
	}
	if err := safehttp.ValidateScheme(req.URL.Scheme); err != nil {
		// Not retryable: the URL will not become valid on the next attempt.
		return attemptResult{Err: fmt.Errorf("%w: %w", errPermanent, err)}
	}
	secret, err := signingSecret(d.Secret)
	if err != nil {
		return attemptResult{Err: fmt.Errorf("%w: cannot sign: %w", errPermanent, err)}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "octarq-webhook-dispatcher/1.0")
	req.Header.Set("X-Octarq-Event", d.Event)
	req.Header.Set("X-Octarq-Delivery", d.DeliveryID)
	req.Header.Set("X-Octarq-Timestamp", strconv.FormatInt(d.SignedAt.Unix(), 10))
	req.Header.Set("X-Octarq-Attempt", strconv.Itoa(attemptNo))
	// v1 (body-only) stays for receivers written against the original scheme.
	// It cannot detect replay on its own — see v2Material.
	req.Header.Set("X-Octarq-Signature", "sha256="+signature(secret, d.Body))
	req.Header.Set("X-Octarq-Signature-V2", "v2="+signature(secret, v2Material(d.SignedAt, d.DeliveryID, d.Body)))

	resp, err := httpClient.Do(req)
	if err != nil {
		return attemptResult{Err: err}
	}
	defer resp.Body.Close()
	// Drain a bounded amount so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return attemptResult{StatusCode: resp.StatusCode, Err: fmt.Errorf("receiver returned HTTP %d", resp.StatusCode)}
	}
	return attemptResult{StatusCode: resp.StatusCode}
}

// errPermanent marks a failure that retrying cannot fix (bad scheme, secret
// that will not decrypt). Such a delivery dead-letters immediately instead of
// burning maxAttempts on a guaranteed failure.
var errPermanent = errors.New("permanent")

// deliverWithRetry runs the attempt/backoff loop for one delivery and keeps the
// persisted log in step with it.
func deliverWithRetry(ctx context.Context, d delivery) {
	ctx, span := telemetry.StartSpan(ctx, "github.com/octarq-org/octarq/eventbus", "eventbus.deliver_webhook",
		trace.WithAttributes(
			attribute.String("webhook.event", d.Event),
			attribute.String("webhook.delivery_id", d.DeliveryID),
			attribute.Int("webhook.id", int(d.WebhookID)),
			attribute.Int("org.id", int(d.OrgID)),
		),
	)
	defer span.End()

	start := time.Now()
	logRow := recordPending(d)

	var last attemptResult
	for n := 1; n <= maxAttempts; n++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		last = attempt(attemptCtx, d, n)
		cancel()

		if last.Err == nil {
			telemetry.SetOK(span)
			telemetry.Global().Metrics.RecordWebhookDelivery(ctx, d.Event, last.StatusCode, time.Since(start))
			recordResult(logRow, n, StatusDelivered, last)
			return
		}
		if errors.Is(last.Err, errPermanent) {
			telemetry.RecordError(span, last.Err)
			telemetry.Global().Metrics.RecordWebhookDelivery(ctx, d.Event, last.StatusCode, time.Since(start))
			log.Printf("eventbus: delivery %s to %s permanently failed: %v", d.DeliveryID, d.URL, last.Err)
			recordResult(logRow, n, StatusFailed, last)
			return
		}
		recordResult(logRow, n, StatusPending, last)
		if n == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			telemetry.RecordError(span, ctx.Err())
			telemetry.Global().Metrics.RecordWebhookDelivery(ctx, d.Event, last.StatusCode, time.Since(start))
			recordResult(logRow, n, StatusFailed, attemptResult{StatusCode: last.StatusCode, Err: ctx.Err()})
			return
		default:
		}
		sleepFn(ctx, backoffFor(n))
	}

	telemetry.RecordError(span, last.Err)
	telemetry.Global().Metrics.RecordWebhookDelivery(ctx, d.Event, last.StatusCode, time.Since(start))
	log.Printf("eventbus: delivery %s to %s dead-lettered after %d attempts: %v", d.DeliveryID, d.URL, maxAttempts, last.Err)
	recordResult(logRow, maxAttempts, StatusFailed, last)
}

// recordPending inserts the delivery log row. A DB failure here must not stop
// the send: losing the audit row is bad, losing the event is worse.
func recordPending(d delivery) *WebhookDelivery {
	row := &WebhookDelivery{
		DeliveryID: d.DeliveryID,
		OrgID:      d.OrgID,
		WebhookID:  d.WebhookID,
		Event:      d.Event,
		URL:        d.URL,
		Body:       string(d.Body),
		Status:     StatusPending,
		SignedAt:   d.SignedAt,
	}
	if db == nil {
		return row
	}
	if err := db.Create(row).Error; err != nil {
		log.Printf("eventbus: could not record delivery %s: %v", d.DeliveryID, err)
	}
	return row
}

func recordResult(row *WebhookDelivery, attempts int, status string, res attemptResult) {
	row.Attempts = attempts
	row.Status = status
	row.ResponseCode = res.StatusCode
	if res.Err != nil {
		row.LastError = truncate(res.Err.Error(), 1024)
	} else {
		row.LastError = ""
	}
	if db == nil || row.ID == 0 {
		return
	}
	err := db.Model(&WebhookDelivery{}).Where("id = ?", row.ID).Updates(map[string]any{
		"attempts":      row.Attempts,
		"status":        row.Status,
		"response_code": row.ResponseCode,
		"last_error":    row.LastError,
	}).Error
	if err != nil {
		log.Printf("eventbus: could not update delivery %s: %v", row.DeliveryID, err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Deliveries returns this org's delivery log, newest first, for the dashboard.
// status is optional ("" = all).
func Deliveries(ctx context.Context, orgID uint, status string, limit int) ([]WebhookDelivery, error) {
	if db == nil {
		return nil, errors.New("eventbus: not initialised")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := db.WithContext(ctx).Where("owner_id = ?", orgID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []WebhookDelivery
	return out, q.Order("id DESC").Limit(limit).Find(&out).Error
}

// Replay re-sends a logged delivery with its ORIGINAL body and delivery id, so
// a receiver that already processed it can dedupe rather than double-handle.
// The signing timestamp is refreshed because the receiver's tolerance window
// would otherwise reject the replay outright.
func Replay(ctx context.Context, orgID uint, deliveryID string) error {
	if db == nil {
		return errors.New("eventbus: not initialised")
	}
	var row WebhookDelivery
	err := db.WithContext(ctx).Where("owner_id = ? AND delivery_id = ?", orgID, deliveryID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("eventbus: delivery %q not found", deliveryID)
		}
		return err
	}
	secret, err := webhookSecret(ctx, row.WebhookID)
	if err != nil {
		return err
	}
	d := delivery{
		DeliveryID: row.DeliveryID,
		OrgID:      row.OrgID,
		WebhookID:  row.WebhookID,
		Event:      row.Event,
		URL:        row.URL,
		Secret:     secret,
		Body:       []byte(row.Body),
		SignedAt:   nowFn(),
	}
	res := attempt(ctx, d, row.Attempts+1)
	status := StatusDelivered
	if res.Err != nil {
		status = StatusFailed
	}
	recordResult(&row, row.Attempts+1, status, res)
	return res.Err
}

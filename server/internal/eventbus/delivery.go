package eventbus

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type WebhookDelivery struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	DeliveryID   string    `gorm:"uniqueIndex;size:64;not null" json:"deliveryId"`
	OrgID        uint      `gorm:"index;column:owner_id;not null" json:"-"`
	WebhookID    uint      `gorm:"index;not null" json:"webhookId"`
	Event        string    `gorm:"size:128;not null" json:"event"`
	URL          string    `gorm:"size:1024;not null" json:"url"`
	Body         string    `gorm:"type:text" json:"-"`
	Status       string    `gorm:"size:16;index;not null;default:'pending'" json:"status"`
	Attempts     int       `gorm:"not null;default:0" json:"attempts"`
	ResponseCode int       `gorm:"not null;default:0" json:"responseCode"`
	LastError    string    `gorm:"size:1024" json:"lastError,omitempty"`
	SignedAt     time.Time `json:"signedAt"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

func Models() []any { return []any{&WebhookDelivery{}} }

var (
	maxAttempts             = 5
	baseBackoff             = 1 * time.Second
	maxBackoff              = 5 * time.Minute
	maxConcurrentDeliveries = 16
	deliverySem             = make(chan struct{}, maxConcurrentDeliveries)

	sleepFn = func(ctx context.Context, d time.Duration) {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
		}
	}
	nowFn         = time.Now
	newDeliveryID = func() string { return uuid.NewString() }
)

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
	half := d / 2
	if half <= 0 {
		return d
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(half)+1))
	if err != nil {
		return d
	}
	return half + time.Duration(n.Int64())
}

func signature(secret string, material []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(material)
	return hex.EncodeToString(mac.Sum(nil))
}

func v2Material(ts time.Time, deliveryID string, body []byte) []byte {
	prefix := strconv.FormatInt(ts.Unix(), 10) + "." + deliveryID + "."
	out := make([]byte, 0, len(prefix)+len(body))
	out = append(out, prefix...)
	return append(out, body...)
}

type delivery struct {
	DeliveryID  string
	OrgID       uint
	WebhookID   uint
	Event       string
	URL         string
	VaultItemID uint
	Body        []byte
	SignedAt    time.Time
}

type attemptResult struct {
	StatusCode int
	Err        error
}

func attempt(ctx context.Context, d delivery, attemptNo int) attemptResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader(d.Body))
	if err != nil {
		return attemptResult{Err: err}
	}
	if err := safehttp.ValidateScheme(req.URL.Scheme); err != nil {
		return attemptResult{Err: fmt.Errorf("%w: %w", errPermanent, err)}
	}
	secret, err := signingSecret(ctx, d.OrgID, d.VaultItemID)
	if err != nil {
		return attemptResult{Err: fmt.Errorf("%w: cannot sign: %w", errPermanent, err)}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "octarq-webhook-dispatcher/1.0")
	req.Header.Set("X-Octarq-Event", d.Event)
	req.Header.Set("X-Octarq-Delivery", d.DeliveryID)
	req.Header.Set("X-Octarq-Timestamp", strconv.FormatInt(d.SignedAt.Unix(), 10))
	req.Header.Set("X-Octarq-Attempt", strconv.Itoa(attemptNo))
	req.Header.Set("X-Octarq-Signature", "sha256="+signature(secret, d.Body))
	req.Header.Set("X-Octarq-Signature-V2", "v2="+signature(secret, v2Material(d.SignedAt, d.DeliveryID, d.Body)))

	resp, err := httpClient.Do(req)
	if err != nil {
		return attemptResult{Err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return attemptResult{StatusCode: resp.StatusCode, Err: fmt.Errorf("receiver returned HTTP %d", resp.StatusCode)}
	}
	return attemptResult{StatusCode: resp.StatusCode}
}

var errPermanent = errors.New("permanent")

func deliverWithRetry(ctx context.Context, d delivery) {
	ctx, span := telemetry.StartSpan(ctx, "github.com/octarq-org/octarq/server/eventbus", "eventbus.deliver_webhook",
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
	gdb := getDB()
	if gdb == nil {
		return row
	}
	if err := gdb.Create(row).Error; err != nil {
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
	if row.ID == 0 {
		return
	}
	gdb := getDB()
	if gdb == nil {
		return
	}
	err := gdb.Model(&WebhookDelivery{}).Where("id = ?", row.ID).Updates(map[string]any{
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

func Deliveries(ctx context.Context, orgID uint, status string, limit int) ([]WebhookDelivery, error) {
	gdb := getDB()
	if gdb == nil {
		return nil, errors.New("eventbus: not initialised")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := gdb.WithContext(ctx).Where("owner_id = ?", orgID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []WebhookDelivery
	return out, q.Order("id DESC").Limit(limit).Find(&out).Error
}

func Replay(ctx context.Context, orgID uint, deliveryID string) error {
	gdb := getDB()
	if gdb == nil {
		return errors.New("eventbus: not initialised")
	}
	var row WebhookDelivery
	err := gdb.WithContext(ctx).Where("owner_id = ? AND delivery_id = ?", orgID, deliveryID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("eventbus: delivery %q not found", deliveryID)
		}
		return err
	}
	vaultItemID, err := webhookVaultItemID(ctx, orgID, row.WebhookID)
	if err != nil {
		return err
	}
	d := delivery{
		DeliveryID:  row.DeliveryID,
		OrgID:       row.OrgID,
		WebhookID:   row.WebhookID,
		Event:       row.Event,
		URL:         row.URL,
		VaultItemID: vaultItemID,
		Body:        []byte(row.Body),
		SignedAt:    nowFn(),
	}
	res := attempt(ctx, d, row.Attempts+1)
	status := StatusDelivered
	if res.Err != nil {
		status = StatusFailed
	}
	recordResult(&row, row.Attempts+1, status, res)
	return res.Err
}

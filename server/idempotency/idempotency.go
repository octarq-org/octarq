package idempotency

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"gorm.io/gorm"
)

type Record struct {
	ID           uint   `gorm:"primaryKey"`
	OrgID        uint   `gorm:"column:owner_id;not null;uniqueIndex:idx_idem_scope,priority:1"`
	Endpoint     string `gorm:"size:255;not null;uniqueIndex:idx_idem_scope,priority:2"`
	Key          string `gorm:"size:255;not null;uniqueIndex:idx_idem_scope,priority:3"`
	RequestHash  string `gorm:"size:64;not null"`
	State        string `gorm:"size:16;not null"`
	StatusCode   int    `gorm:"not null;default:0"`
	ContentType  string `gorm:"size:128"`
	BodyStored   bool   `gorm:"not null;default:0"`
	ResponseBody []byte
	CreatedAt    time.Time
	ExpiresAt    time.Time `gorm:"index;not null"`
}

func (Record) TableName() string { return "idempotency_records" }

const (
	stateInFlight = "in_flight"
	stateDone     = "done"
)

func Models() []any { return []any{&Record{}} }

const ServiceName = "core.idempotency"

const HeaderKey = "Idempotency-Key"

const HeaderReplayed = "Idempotency-Replayed"

type Store struct {
	db      *gorm.DB
	ttl     time.Duration
	maxBody int64
	now     func() time.Time
}

type Option func(*Store)

func WithTTL(d time.Duration) Option { return func(s *Store) { s.ttl = d } }

func WithMaxBody(n int64) Option { return func(s *Store) { s.maxBody = n } }

func WithClock(fn func() time.Time) Option { return func(s *Store) { s.now = fn } }

func New(db *gorm.DB, opts ...Option) *Store {
	s := &Store{db: db, ttl: 24 * time.Hour, maxBody: 1 << 20, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

type OrgFunc func(*http.Request) uint

func (s *Store) Middleware(orgOf OrgFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.serve(w, r, orgOf, next)
		})
	}
}

func HumaMiddleware(idem func(http.Handler) http.Handler) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		r, w := humago.Unwrap(ctx)
		idem(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next(ctx)
		})).ServeHTTP(w, r)
	}
}

func unsafeMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func (s *Store) serve(w http.ResponseWriter, r *http.Request, orgOf OrgFunc, next http.Handler) {
	key := r.Header.Get(HeaderKey)
	if key == "" || !unsafeMethod(r.Method) || s.db == nil {
		next.ServeHTTP(w, r)
		return
	}
	if len(key) > 255 {
		writeErr(w, http.StatusBadRequest, "Idempotency-Key must be at most 255 characters")
		return
	}
	var orgID uint
	if orgOf != nil {
		orgID = orgOf(r)
	}
	if orgID == 0 {
		next.ServeHTTP(w, r)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, s.maxBody+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read request body")
		return
	}
	if int64(len(body)) > s.maxBody {
		writeErr(w, http.StatusRequestEntityTooLarge, "request too large for idempotent replay")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	endpoint := r.Method + " " + r.URL.Path
	hash := requestHash(body)
	now := s.now()

	rec, outcome := s.claim(orgID, endpoint, key, hash, now)
	switch outcome {
	case claimReplay:
		replay(w, rec)
		return
	case claimConflict:
		w.Header().Set("Retry-After", "1")
		writeErr(w, http.StatusConflict, "a request with this Idempotency-Key is still in progress")
		return
	case claimMismatch:
		writeErr(w, http.StatusUnprocessableEntity, "this Idempotency-Key was already used with a different request body")
		return
	case claimError:
		slog.Error("idempotency: could not claim key, serving without de-duplication", "endpoint", endpoint)
		next.ServeHTTP(w, r)
		return
	}

	captured := &captureWriter{ResponseWriter: w, limit: s.maxBody}
	committed := false
	defer func() {
		if !committed {
			s.release(rec.ID)
		}
	}()
	next.ServeHTTP(captured, r)

	status := captured.status
	if status == 0 {
		status = http.StatusOK
	}
	if status >= 500 || status == http.StatusTooManyRequests {
		return
	}
	committed = true
	body, stored := captured.body()
	s.commit(rec.ID, status, captured.Header().Get("Content-Type"), body, stored, now.Add(s.ttl))
}

type claimOutcome int

const (
	claimOwned claimOutcome = iota
	claimReplay
	claimConflict
	claimMismatch
	claimError
)

func (s *Store) claim(orgID uint, endpoint, key, hash string, now time.Time) (*Record, claimOutcome) {
	rec := &Record{
		OrgID: orgID, Endpoint: endpoint, Key: key, RequestHash: hash,
		State: stateInFlight, CreatedAt: now, ExpiresAt: now.Add(s.ttl),
	}
	err := s.db.Create(rec).Error
	if err == nil {
		return rec, claimOwned
	}

	var existing Record
	if lookup := s.db.Where("owner_id = ? AND endpoint = ? AND key = ?", orgID, endpoint, key).First(&existing).Error; lookup != nil {
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			slog.Error("idempotency: key lookup failed", "err", lookup)
		}
		return nil, claimError
	}
	if !existing.ExpiresAt.After(now) {
		upd := s.db.Model(&Record{}).Where("id = ? AND expires_at = ?", existing.ID, existing.ExpiresAt).Updates(map[string]any{
			"request_hash":  hash,
			"state":         stateInFlight,
			"status_code":   0,
			"content_type":  "",
			"response_body": nil,
			"created_at":    now,
			"expires_at":    now.Add(s.ttl),
		})
		if upd.Error != nil || upd.RowsAffected == 0 {
			return nil, claimError
		}
		return &Record{ID: existing.ID}, claimOwned
	}
	if existing.RequestHash != hash {
		return nil, claimMismatch
	}
	if existing.State == stateDone {
		return &existing, claimReplay
	}
	return nil, claimConflict
}

func (s *Store) commit(id uint, status int, contentType string, body []byte, stored bool, expires time.Time) {
	err := s.db.Model(&Record{}).Where("id = ?", id).Updates(map[string]any{
		"state":         stateDone,
		"status_code":   status,
		"content_type":  contentType,
		"body_stored":   stored,
		"response_body": body,
		"expires_at":    expires,
	}).Error
	if err != nil {
		slog.Error("idempotency: could not store response for replay", "err", err)
	}
}

func (s *Store) release(id uint) {
	if err := s.db.Delete(&Record{}, id).Error; err != nil {
		slog.Error("idempotency: could not release key", "err", err)
	}
}

func (s *Store) Purge() (int64, error) {
	res := s.db.Where("expires_at <= ?", s.now()).Delete(&Record{})
	return res.RowsAffected, res.Error
}

func requestHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func replay(w http.ResponseWriter, rec *Record) {
	if !rec.BodyStored {
		w.Header().Set("Idempotency-Original-Status", strconv.Itoa(rec.StatusCode))
		writeErr(w, http.StatusConflict, "the original request with this Idempotency-Key already completed; its response was not stored for replay")
		return
	}
	if ct := rec.ContentType; ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set(HeaderReplayed, "true")
	w.Header().Set("Content-Length", strconv.Itoa(len(rec.ResponseBody)))
	w.WriteHeader(rec.StatusCode)
	if _, err := w.Write(rec.ResponseBody); err != nil {
		slog.Warn("idempotency: failed to write cached response body", "err", err)
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		slog.Warn("idempotency: failed to encode error response", "err", err)
	}
}

type captureWriter struct {
	http.ResponseWriter
	status   int
	buf      bytes.Buffer
	limit    int64
	overflow bool
}

func (c *captureWriter) WriteHeader(code int) {
	if c.status == 0 {
		c.status = code
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if !c.overflow {
		if int64(c.buf.Len()+len(b)) > c.limit {
			c.overflow = true
			c.buf.Reset()
		} else {
			c.buf.Write(b)
		}
	}
	return c.ResponseWriter.Write(b)
}

func (c *captureWriter) Flush() {
	c.overflow = true
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *captureWriter) body() ([]byte, bool) {
	if c.overflow {
		return nil, false
	}
	return c.buf.Bytes(), true
}

// Package idempotency makes unsafe HTTP endpoints safe to retry.
//
// The problem it solves: a client POSTs "send this email" or "create this
// link", the response is lost to a timeout or a dropped connection, and the
// client retries. Without a de-duplication key the server has no way to know
// the second request is the same intent as the first, so the mail goes out
// twice and the resource is created twice.
//
// The contract (deliberately the one Stripe popularised, because integrators
// already know it):
//
//   - The client sends `Idempotency-Key: <opaque unique string>` on an unsafe
//     request. Requests without the header are untouched — adoption is
//     per-client and per-endpoint, never a silent behaviour change.
//   - The key is scoped to (org, endpoint, key). Two workspaces, or two
//     endpoints, may use the same key string without colliding; the same key
//     on the same endpoint in the same workspace is the same operation.
//   - The FIRST request runs the handler and its response is stored. A repeat
//     within the TTL is answered from the store, byte for byte, with
//     `Idempotency-Replayed: true` — the handler never runs twice.
//   - A repeat that arrives while the first is still in flight gets 409 with
//     Retry-After, not a second execution.
//   - Reusing a key with a DIFFERENT request body is a client bug and is
//     rejected with 422 rather than silently replaying an unrelated response.
//
// Failures are deliberately NOT cached: a 5xx, 429, or a panic releases the
// key so the client's retry can actually be attempted. Caching a transient
// failure would make the retry the client is entitled to impossible.
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

// Record is one claimed idempotency key and, once the handler has finished,
// the response to replay for repeats of it.
type Record struct {
	ID uint `gorm:"primaryKey"`
	// The uniqueness scope. OrgID uses the repo-wide owner_id column name.
	OrgID    uint   `gorm:"column:owner_id;not null;uniqueIndex:idx_idem_scope,priority:1"`
	Endpoint string `gorm:"size:255;not null;uniqueIndex:idx_idem_scope,priority:2"`
	Key      string `gorm:"size:255;not null;uniqueIndex:idx_idem_scope,priority:3"`
	// RequestHash pins the key to one request payload, so a client that reuses
	// a key for a different operation is told rather than served someone
	// else's answer.
	RequestHash string `gorm:"size:64;not null"`
	// State is inFlight until the handler returns, then done.
	State       string `gorm:"size:16;not null"`
	StatusCode  int    `gorm:"not null;default:0"`
	ContentType string `gorm:"size:128"`
	// BodyStored distinguishes "the response was empty" from "the response
	// could not be captured" (too large, or streamed). Replaying an
	// uncaptured response as an empty body would hand the client a wrong
	// answer, so that case is reported instead — see replay.
	BodyStored   bool   `gorm:"not null;default:0"`
	ResponseBody []byte `gorm:"type:blob"`
	CreatedAt    time.Time
	ExpiresAt    time.Time `gorm:"index;not null"`
}

// TableName keeps the table name explicit rather than derived, since the
// struct name is generic.
func (Record) TableName() string { return "idempotency_records" }

const (
	stateInFlight = "in_flight"
	stateDone     = "done"
)

// Models returns this package's models for the app's single AutoMigrate pass.
func Models() []any { return []any{&Record{}} }

// ServiceName is how the middleware is published in the plugin service
// registry (plugin.Context.Lookup). It lives here, not in the plugin package,
// so an out-of-tree plugin can resolve the seam by importing one package:
//
//	if idem, ok := plugin.LookupServiceAs[func(http.Handler) http.Handler](
//		ctx.Lookup, idempotency.ServiceName); ok {
//		mux.Handle("POST /api/x/mine/things", idem(ctx.Guard(createThing)))
//	}
//
// The lookup is deliberately optional: a composition that does not provide it
// (an embedder wiring its own app) leaves the route working, just without
// de-duplication.
const ServiceName = "core.idempotency"

// HeaderKey is the request header clients send.
const HeaderKey = "Idempotency-Key"

// HeaderReplayed marks a response served from the store rather than freshly
// executed.
const HeaderReplayed = "Idempotency-Replayed"

// Store persists idempotency records.
type Store struct {
	db  *gorm.DB
	ttl time.Duration
	// maxBody bounds how much of a request we hash and how much of a response
	// we retain. A response larger than this is served normally but NOT
	// stored — replaying a truncated body would be worse than not replaying.
	maxBody int64
	now     func() time.Time
}

// Option customises a Store.
type Option func(*Store)

// WithTTL sets how long a completed response stays replayable (default 24h).
func WithTTL(d time.Duration) Option { return func(s *Store) { s.ttl = d } }

// WithMaxBody sets the request/response size ceiling in bytes (default 1 MiB).
func WithMaxBody(n int64) Option { return func(s *Store) { s.maxBody = n } }

// WithClock replaces time.Now (tests).
func WithClock(fn func() time.Time) Option { return func(s *Store) { s.now = fn } }

// New returns a Store backed by db.
func New(db *gorm.DB, opts ...Option) *Store {
	s := &Store{db: db, ttl: 24 * time.Hour, maxBody: 1 << 20, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

// OrgFunc resolves the calling workspace for a request. Returning 0 means "no
// workspace in this request"; such requests bypass the mechanism rather than
// sharing one global key namespace across tenants.
type OrgFunc func(*http.Request) uint

// Middleware returns the wrapper plugin routes adopt:
//
//	mux.Handle("POST /api/links", idem(guard(createLink)))
//
// It is a plain func(http.Handler) http.Handler so it composes with the
// existing guard/gate wrappers and can be handed across the plugin service
// registry without a shared interface type.
func (s *Store) Middleware(orgOf OrgFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.serve(w, r, orgOf, next)
		})
	}
}

// HumaMiddleware adapts an http.Handler idempotency middleware into a huma.Middleware
// so huma operations (such as POST /api/emails/send) can adopt idempotency.
func HumaMiddleware(idem func(http.Handler) http.Handler) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		r, w := humago.Unwrap(ctx)
		idem(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next(ctx)
		})).ServeHTTP(w, r)
	}
}

// unsafeMethod reports whether a method can have side effects worth
// de-duplicating. GET/HEAD/OPTIONS are already idempotent by definition.
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
		// Unscoped request: no tenant to scope the key to. Serve it, but never
		// file it under a namespace shared with every other tenant.
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
		// The store is unavailable. Refusing the request would turn a
		// bookkeeping outage into an outage of the endpoint itself; serving it
		// un-deduplicated is the same behaviour as a client that sent no key.
		slog.Error("idempotency: could not claim key, serving without de-duplication", "endpoint", endpoint)
		next.ServeHTTP(w, r)
		return
	}

	// We own the key. Release it if the handler panics so the client's retry
	// is not permanently answered with "still in progress".
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
	// Transient failures release the key: the client is entitled to retry, and
	// a cached 503 would deny it forever.
	if status >= 500 || status == http.StatusTooManyRequests {
		return
	}
	// The key stays claimed even when the body could not be captured: the
	// handler DID run, and re-running it is the one outcome this package
	// exists to prevent.
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

// claim inserts an in-flight record, or reports what the existing one means.
// The unique index on (owner_id, endpoint, key) is what makes this atomic
// across concurrent requests and across processes — two app instances behind a
// load balancer race on the INSERT and exactly one wins.
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
	// The insert lost the race (or failed outright). If a record exists, the
	// cause was the unique index and the existing row decides the outcome;
	// anything else is a store problem.
	if lookup := s.db.Where("owner_id = ? AND endpoint = ? AND key = ?", orgID, endpoint, key).First(&existing).Error; lookup != nil {
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			slog.Error("idempotency: key lookup failed", "err", lookup)
		}
		return nil, claimError
	}
	if !existing.ExpiresAt.After(now) {
		// Expired: reclaim it in place for this request.
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

// Purge deletes expired records. Call it periodically; nothing depends on it
// for correctness (an expired record is never replayed), it only bounds table
// growth.
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
		// The original ran and succeeded, but its response was streamed or too
		// large to keep. Saying so is the only honest answer: replaying an
		// empty body would look like a real (wrong) response, and re-running
		// the handler would double the side effect.
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
	_, _ = w.Write(rec.ResponseBody)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// captureWriter tees the handler's response so it can be stored for replay.
// Writes always reach the client first: capture is bookkeeping and must never
// change what this caller sees.
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

// Flush keeps streaming handlers working. A flushed response has already left
// the building, so it is not stored.
func (c *captureWriter) Flush() {
	c.overflow = true
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// body returns the captured response and whether it is complete. An
// incomplete capture is never stored as if it were the response.
func (c *captureWriter) body() ([]byte, bool) {
	if c.overflow {
		return nil, false
	}
	return c.buf.Bytes(), true
}

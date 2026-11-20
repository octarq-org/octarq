package sse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultHeartbeat  = 15 * time.Second
	DefaultBuffer     = 16
	DefaultTenantCap  = 32
	DefaultMaxEventID = 256
)

var (
	ErrNoAuth         = errors.New("sse: request is not authenticated")
	ErrCSRF           = errors.New("sse: origin check failed")
	ErrTenantFull     = errors.New("sse: tenant connection limit reached")
	ErrZeroOrg        = errors.New("sse: connection requires a non-zero org id")
	ErrEventIDTooBig  = errors.New("sse: event id exceeds maximum length")
	ErrEventIDInvalid = errors.New("sse: event id contains control characters")
)

type AuthFunc func(r *http.Request) (uint64, error)

type CSRFVerify func(r *http.Request, orgID uint64) error

type Conn struct {
	orgID   uint64
	queue   chan string
	dropped atomic.Int64
	closed  atomic.Bool
	done    chan struct{}
	once    sync.Once
}

func (c *Conn) OrgID() uint64 { return c.orgID }

func (c *Conn) Dropped() int64 { return c.dropped.Load() }

func (c *Conn) Done() <-chan struct{} { return c.done }

func (c *Conn) Queue() <-chan string { return c.queue }

func (c *Conn) Close() {
	c.close()
}

func (c *Conn) close() {
	c.once.Do(func() {
		c.closed.Store(true)
		close(c.done)
	})
}

func Prepare(w http.ResponseWriter) (flusher http.Flusher, ok bool) {
	flusher, ok = w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return flusher, true
}

func CommentFrame(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return ": " + s + "\n\n"
}

func IDFrame(id string) string {
	id = strings.ReplaceAll(id, "\n", " ")
	id = strings.ReplaceAll(id, "\r", " ")
	return fmt.Sprintf("id: %s\n\n", id)
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

type Hub struct {
	mu        sync.RWMutex
	subs      map[uint64]map[*Conn]struct{}
	auth      AuthFunc
	csrf      CSRFVerify
	origins   []string
	heartbeat time.Duration
	buffer    int
	tenantCap int
	maxIDLen  int
	now       func() time.Time
	dropped   atomic.Int64
}

type Option func(*Hub)

func WithAuth(fn AuthFunc) Option {
	return func(h *Hub) {
		if fn != nil {
			h.auth = fn
		}
	}
}

func WithOrigins(origins ...string) Option {
	return func(h *Hub) { h.origins = origins }
}

func WithCSRFVerify(fn CSRFVerify) Option {
	return func(h *Hub) {
		if fn != nil {
			h.csrf = fn
		}
	}
}

func WithHeartbeat(d time.Duration) Option {
	return func(h *Hub) {
		if d > 0 {
			h.heartbeat = d
		}
	}
}

func WithBuffer(n int) Option {
	return func(h *Hub) {
		if n > 0 {
			h.buffer = n
		}
	}
}

func WithTenantCap(n int) Option {
	return func(h *Hub) {
		if n > 0 {
			h.tenantCap = n
		}
	}
}

func WithMaxEventID(n int) Option {
	return func(h *Hub) {
		if n > 0 {
			h.maxIDLen = n
		}
	}
}

func New(opts ...Option) *Hub {
	h := &Hub{
		subs:      make(map[uint64]map[*Conn]struct{}),
		heartbeat: DefaultHeartbeat,
		buffer:    DefaultBuffer,
		tenantCap: DefaultTenantCap,
		maxIDLen:  DefaultMaxEventID,
		now:       time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}
	return h
}

func (h *Hub) Register(orgID uint64) (*Conn, error) {
	if orgID == 0 {
		return nil, ErrZeroOrg
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.subs[orgID]
	if len(set) >= h.tenantCap {
		return nil, ErrTenantFull
	}
	c := &Conn{
		orgID: orgID,
		queue: make(chan string, h.buffer),
		done:  make(chan struct{}),
	}
	if set == nil {
		set = make(map[*Conn]struct{})
		h.subs[orgID] = set
	}
	set[c] = struct{}{}
	return c, nil
}

func (h *Hub) Unregister(c *Conn) {
	if c == nil {
		return
	}
	c.close()
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[c.orgID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.subs, c.orgID)
		}
	}
}

func (h *Hub) Connections(orgID uint64) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[orgID])
}

func (h *Hub) Broadcast(orgID uint64, eventID string) error {
	if orgID == 0 {
		return ErrZeroOrg
	}
	if len(eventID) > h.maxIDLen {
		return fmt.Errorf("%w: %d bytes", ErrEventIDTooBig, len(eventID))
	}
	if strings.TrimSpace(eventID) == "" {
		return fmt.Errorf("%w: event id is required", ErrEventIDTooBig)
	}
	if hasControl(eventID) {
		return ErrEventIDInvalid
	}
	h.mu.RLock()
	set := make([]*Conn, 0, len(h.subs[orgID]))
	for c := range h.subs[orgID] {
		set = append(set, c)
	}
	h.mu.RUnlock()

	for _, c := range set {
		if c.closed.Load() {
			continue
		}
		select {
		case c.queue <- eventID:
		default:
			c.dropped.Add(1)
			h.dropped.Add(1)
		}
	}
	return nil
}

func (h *Hub) Dropped() int64 { return h.dropped.Load() }

func (h *Hub) checkOrigin(r *http.Request) error {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return nil
	}
	for _, suffix := range h.origins {
		suffix = strings.TrimSpace(suffix)
		if suffix == "" {
			continue
		}
		if !strings.HasPrefix(suffix, ".") && !strings.Contains(suffix, "://") {
			continue
		}
		if strings.HasSuffix(strings.ToLower(origin), strings.ToLower(suffix)) {
			return nil
		}
	}
	return fmt.Errorf("%w: %q is not allowlisted", ErrCSRF, origin)
}

func (h *Hub) Subscribe(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		http.Error(w, ErrNoAuth.Error(), http.StatusUnauthorized)
		return
	}
	orgID, err := h.auth(r)
	if err != nil || orgID == 0 {
		http.Error(w, ErrNoAuth.Error(), http.StatusUnauthorized)
		return
	}
	if h.csrf != nil {
		if err := h.csrf(r, orgID); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}
	if err := h.checkOrigin(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	c, err := h.Register(orgID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrTenantFull) {
			status = http.StatusTooManyRequests
		}
		http.Error(w, err.Error(), status)
		return
	}
	defer h.Unregister(c)

	flusher, ok := Prepare(w)
	if !ok {
		http.Error(w, "sse: streaming is not supported", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, CommentFrame("connected"))
	flusher.Flush()

	heartbeat := time.NewTicker(h.heartbeat)
	defer heartbeat.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case id := <-c.queue:
			_, _ = io.WriteString(w, IDFrame(id))
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = io.WriteString(w, CommentFrame("ping"))
			flusher.Flush()
		}
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Subscribe(w, r)
}

func (h *Hub) Close() {
	h.Drain(context.Background())
}

func (h *Hub) Drain(_ context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		for c := range set {
			c.close()
			delete(set, c)
		}
	}
	h.subs = make(map[uint64]map[*Conn]struct{})
}

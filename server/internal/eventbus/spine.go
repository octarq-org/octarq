package eventbus

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// Envelope is the typed event carrier for the in-process event spine.
type Envelope = plugin.Envelope

// SubscribeOpts controls the channel buffer and key filter for a subscription.
type SubscribeOpts struct {
	BufferSize int      // default 64; <=0 uses the default
	Keys       []string // empty = receive all; non-empty = only matching Key values
}

const defaultSpineBuffer = 64

// spineSub is an internal subscription entry.
//
// Concurrency contract
// ────────────────────
//   - spMu (global RWMutex) guards the spSubs slice.
//   - subMu (per-sub Mutex) guards closed and the drain-modify-refill slow path.
//
// Ordering rule: acquire subMu ONLY while NOT holding spMu, or acquire spMu
// FIRST and subMu SECOND — never the other way around.
//
// cancel removes the sub from spSubs under spMu.Lock(), then sets closed=true
// and closes the channel under subMu.Lock(). spineSend acquires subMu to check
// closed before any channel operation, so it can never send on a closed channel.
type spineSub struct {
	ch     chan Envelope
	keys   map[string]struct{} // nil = match all; immutable after Subscribe returns
	subMu  sync.Mutex          // guards closed; also serialises drain-refill slow path
	closed bool                // set to true exactly once, inside subMu
	once   sync.Once           // ensures cancel body runs exactly once
}

var (
	spMu       sync.RWMutex // guards spSubs slice
	spSubs     []*spineSub
	droppedCnt uint64
)

// Subscribe returns a receive-only Envelope channel and an idempotent cancel
// function. cancel removes the subscription and closes the channel exactly once;
// subsequent calls are no-ops.
func Subscribe(opts SubscribeOpts) (<-chan Envelope, func()) {
	size := opts.BufferSize
	if size <= 0 {
		size = defaultSpineBuffer
	}

	var keySet map[string]struct{}
	if len(opts.Keys) > 0 {
		keySet = make(map[string]struct{}, len(opts.Keys))
		for _, k := range opts.Keys {
			keySet[k] = struct{}{}
		}
	}

	s := &spineSub{
		ch:   make(chan Envelope, size),
		keys: keySet,
	}

	spMu.Lock()
	spSubs = append(spSubs, s)
	spMu.Unlock()

	cancel := func() {
		s.once.Do(func() {
			// Step 1: remove from the global list so no new publisher can see this sub.
			spMu.Lock()
			for i, existing := range spSubs {
				if existing == s {
					spSubs[i] = spSubs[len(spSubs)-1]
					spSubs[len(spSubs)-1] = nil
					spSubs = spSubs[:len(spSubs)-1]
					break
				}
			}
			spMu.Unlock()

			// Step 2: mark closed and close the channel under subMu so that any
			// spineSend that already took a local pointer to s (before step 1
			// removed it from the list) will see closed=true and bail out before
			// touching the channel.
			s.subMu.Lock()
			s.closed = true
			close(s.ch)
			s.subMu.Unlock()
		})
	}

	return s.ch, cancel
}

// PublishEnvelope validates OrgID and Key, auto-fills ID/OccurredAt when zero,
// then fans the envelope out to all matching subscribers without ever blocking.
func PublishEnvelope(env Envelope) {
	if env.OrgID == 0 || env.Key == "" {
		return
	}
	if env.ID == "" {
		env.ID = spineRandHex(16)
	}
	if env.OccurredAt.IsZero() {
		env.OccurredAt = time.Now()
	}

	// Take a snapshot of the subscriber slice so we don't hold spMu during
	// (potentially slow) channel operations.
	spMu.RLock()
	subs := make([]*spineSub, len(spSubs))
	copy(subs, spSubs)
	spMu.RUnlock()

	for _, s := range subs {
		// Key filter is immutable; check without a lock.
		if s.keys != nil {
			if _, ok := s.keys[env.Key]; !ok {
				continue
			}
		}
		spineSend(s, env)
	}
}

// spineSend delivers env to s without ever blocking the caller.
//
// Fast path: non-blocking send under subMu, checking closed first.
// Slow path: channel full → drain-modify-refill under the same lock.
//
// We always hold subMu before touching s.ch. This serialises with cancel, which
// sets s.closed = true and closes s.ch under the same lock, so we can never
// send on a closed channel.
func spineSend(s *spineSub, env Envelope) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	if s.closed {
		return
	}

	// Fast path: channel has room.
	select {
	case s.ch <- env:
		return
	default:
	}

	// Slow path: channel full — drain-modify-refill.

	// Drain everything currently buffered.
	n := len(s.ch)
	buffered := make([]Envelope, 0, n)
	for i := 0; i < n; i++ {
		select {
		case e := <-s.ch:
			buffered = append(buffered, e)
		default:
		}
	}

	// Drop oldest item matching env.EntityKey; fallback to overall head.
	dropped := false
	if env.EntityKey != "" {
		for i, e := range buffered {
			if e.EntityKey == env.EntityKey {
				buffered = append(buffered[:i], buffered[i+1:]...)
				dropped = true
				break
			}
		}
	}
	if !dropped && len(buffered) > 0 {
		buffered = buffered[1:]
		dropped = true
	}
	if dropped {
		atomic.AddUint64(&droppedCnt, 1)
	}

	// Refill retained items.
	for _, e := range buffered {
		select {
		case s.ch <- e:
		default:
			// Defensive: shouldn't happen since cap > len(retained).
			atomic.AddUint64(&droppedCnt, 1)
		}
	}
	// Deliver the new envelope — must succeed now.
	select {
	case s.ch <- env:
	default:
		atomic.AddUint64(&droppedCnt, 1)
	}
}

// DroppedTotal returns the cumulative count of envelopes dropped due to
// subscriber backpressure. Use as a metrics/alerting seam.
func DroppedTotal() uint64 {
	return atomic.LoadUint64(&droppedCnt)
}

// spineRandHex returns n random bytes encoded as a lowercase hex string.
func spineRandHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

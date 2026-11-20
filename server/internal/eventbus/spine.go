package eventbus

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

type Envelope = plugin.Envelope

type SubscribeOpts struct {
	BufferSize int
	Keys       []string
}

const defaultSpineBuffer = 64

type spineSub struct {
	ch     chan Envelope
	keys   map[string]struct{}
	subMu  sync.Mutex
	closed bool
	once   sync.Once
}

var (
	spMu       sync.RWMutex
	spSubs     []*spineSub
	droppedCnt uint64
)

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

			s.subMu.Lock()
			s.closed = true
			close(s.ch)
			s.subMu.Unlock()
		})
	}

	return s.ch, cancel
}

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

	spMu.RLock()
	subs := make([]*spineSub, len(spSubs))
	copy(subs, spSubs)
	spMu.RUnlock()

	for _, s := range subs {
		if s.keys != nil {
			if _, ok := s.keys[env.Key]; !ok {
				continue
			}
		}
		spineSend(s, env)
	}
}

func spineSend(s *spineSub, env Envelope) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	if s.closed {
		return
	}

	select {
	case s.ch <- env:
		return
	default:
	}

	n := len(s.ch)
	buffered := make([]Envelope, 0, n)
	for i := 0; i < n; i++ {
		select {
		case e := <-s.ch:
			buffered = append(buffered, e)
		default:
		}
	}

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

	for _, e := range buffered {
		select {
		case s.ch <- e:
		default:
			atomic.AddUint64(&droppedCnt, 1)
		}
	}
	select {
	case s.ch <- env:
	default:
		atomic.AddUint64(&droppedCnt, 1)
	}
}

func DroppedTotal() uint64 {
	return atomic.LoadUint64(&droppedCnt)
}

func spineRandHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

package mcp

import (
	"context"
	"errors"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type subscriptionTracker struct {
	mu   sync.Mutex
	subs map[string]int
}

func (t *subscriptionTracker) add(uri string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.subs == nil {
		t.subs = map[string]int{}
	}
	t.subs[uri]++
}

func (t *subscriptionTracker) remove(uri string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.subs == nil {
		return
	}
	if t.subs[uri] <= 1 {
		delete(t.subs, uri)
		return
	}
	t.subs[uri]--
}

func (t *subscriptionTracker) watched(uri string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.subs[uri] > 0
}

func (s *server) handleSubscribe(_ context.Context, req *mcp.SubscribeRequest) error {
	if req == nil || req.Params == nil || req.Params.URI == "" {
		return errors.New("missing subscribe URI")
	}
	if s.subs == nil {
		s.subs = &subscriptionTracker{}
	}
	s.subs.add(req.Params.URI)
	return nil
}

func (s *server) handleUnsubscribe(_ context.Context, req *mcp.UnsubscribeRequest) error {
	if req == nil || req.Params == nil || req.Params.URI == "" {
		return errors.New("missing unsubscribe URI")
	}
	if s.subs == nil {
		return nil
	}
	s.subs.remove(req.Params.URI)
	return nil
}

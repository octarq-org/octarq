package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Envelope struct {
	ID         string          `json:"id"`
	OrgID      uint            `json:"orgId"`
	Key        string          `json:"key"`
	EntityKey  string          `json:"entityKey,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	OccurredAt time.Time       `json:"occurredAt"`
}

type EventReactor interface {
	Events() []string
	React(ctx context.Context, env Envelope) error
}

type DebounceReactor interface {
	MinInterval() time.Duration
}

type EntityKeyReactor interface {
	EntityKey(env Envelope) string
}

func RegisterReactor(ctx *Context, r EventReactor) error {
	if r == nil {
		return errors.New("event reactor cannot be nil")
	}
	events := r.Events()
	if len(events) == 0 {
		return errors.New("event reactor must declare at least one event key in Events()")
	}
	for _, k := range events {
		if strings.TrimSpace(k) == "" {
			return errors.New("event key cannot be empty")
		}
	}
	if ctx == nil || ctx.RegisterReactor == nil {
		return nil
	}
	return ctx.RegisterReactor(r)
}

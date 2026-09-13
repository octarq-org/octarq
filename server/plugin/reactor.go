package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Envelope is the typed event carrier passed to reactors and published to the event spine.
type Envelope struct {
	ID         string          `json:"id"`
	OrgID      uint            `json:"orgId"`
	Key        string          `json:"key"`
	EntityKey  string          `json:"entityKey,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	OccurredAt time.Time       `json:"occurredAt"`
}

// EventReactor represents a declarative event reactor declared against the event spine.
type EventReactor interface {
	Events() []string                              // 声明关心的 Key 集合，如 ["link.click","link.anomaly"]
	React(ctx context.Context, env Envelope) error // 幂等处理，失败返回 error 供重试/日志
}

// DebounceReactor is an optional extension interface to configure minimum interval between executions per EntityKey.
type DebounceReactor interface {
	MinInterval() time.Duration
}

// EntityKeyReactor is an optional extension interface to extract a custom EntityKey from the Envelope.
// If not implemented, env.EntityKey is used by default.
type EntityKeyReactor interface {
	EntityKey(env Envelope) string
}

// RegisterReactor is a type-safe helper to validate and register an EventReactor onto a Context.
// If ctx is nil or ctx.RegisterReactor is nil, returns nil for backward compatibility.
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

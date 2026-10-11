package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// ctxKey is an unexported type for context keys defined in this package, so
// they can never collide with keys defined elsewhere.
type ctxKey int

// RequestIDKey is the context key under which the per-request ID is stored.
// Future handlers can read it with r.Context().Value(server.RequestIDKey).
const RequestIDKey ctxKey = iota

// RequestID returns the request ID carried on the context, or "" if absent.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(RequestIDKey).(string); ok {
		return v
	}
	return ""
}

// Request ID

// newRequestID returns a random 16-hex-char (8-byte) request ID.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

// sanitizeRequestID accepts an inbound X-Request-Id only if it's short and made
// of safe characters, otherwise returns "" so we generate a fresh one.
func sanitizeRequestID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 128 {
		return ""
	}
	for _, c := range v {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' && c != '.' {
			return ""
		}
	}
	return v
}

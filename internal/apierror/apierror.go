// Package apierror defines the single error envelope every octarq API response
// uses, and the closed registry of machine-readable codes that go in it.
//
// Why one envelope: an integrator cannot see our code. The only thing they can
// branch on is what we send back, so it has to be the same shape from every
// endpoint, on every path — huma-validated requests, the CSRF guard, the plugin
// feature gate, and a panic alike. Before this package a single request could
// come back as an RFC 7807 document, as {"error":"..."}, or as a bare string,
// depending on which layer refused it.
//
// The envelope is deliberately NOT RFC 7807: `detail` there is prose, and prose
// is what integrators end up string-matching when nothing else discriminates.
// Code is the discriminator; Message is for humans; RequestID is what support
// traces.
//
// It lives in its own leaf package (not internal/api) so the layers that refuse
// requests before the API handler ever runs — the CSRF guard, auth middleware,
// the app's plugin gate and panic recovery — can all produce it without
// importing the API package.
package apierror

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/server"
)

// The closed set of machine-readable error codes. These are API surface: a
// client may branch on them, so a code is added additively and never renamed or
// removed without the deprecation process. The prose in Message is NOT stable
// and must never be matched on.
const (
	CodeBadRequest          = "bad_request"
	CodeValidationFailed    = "validation_failed"
	CodeUnauthorized        = "unauthorized"
	CodeForbidden           = "forbidden"
	CodeCSRFOriginBlocked   = "csrf_origin_blocked"
	CodeCSRFTokenInvalid    = "csrf_token_invalid"
	CodeNotFound            = "not_found"
	CodePluginNotEnabled    = "plugin_not_enabled"
	CodeMethodNotAllowed    = "method_not_allowed"
	CodeConflict            = "conflict"
	CodeUnsupportedMedia    = "unsupported_media_type"
	CodeUnprocessableEntity = "unprocessable_entity"
	CodeRateLimitExceeded   = "rate_limit_exceeded"
	CodeInternalError       = "internal_error"
	CodeNotImplemented      = "not_implemented"
	CodeServiceUnavailable  = "service_unavailable"
	CodeGatewayTimeout      = "gateway_timeout"
)

// codes is the registry. Keeping it in one slice is what lets the spec document
// the enum and lets TestErrorCodeRegistryIsClosed refuse an undocumented code.
var codes = []string{
	CodeBadRequest,
	CodeValidationFailed,
	CodeUnauthorized,
	CodeForbidden,
	CodeCSRFOriginBlocked,
	CodeCSRFTokenInvalid,
	CodeNotFound,
	CodePluginNotEnabled,
	CodeMethodNotAllowed,
	CodeConflict,
	CodeUnsupportedMedia,
	CodeUnprocessableEntity,
	CodeRateLimitExceeded,
	CodeInternalError,
	CodeNotImplemented,
	CodeServiceUnavailable,
	CodeGatewayTimeout,
}

// Codes returns the registry, sorted, for the spec's enum and for tests.
func Codes() []string {
	out := append([]string(nil), codes...)
	sort.Strings(out)
	return out
}

// Known reports whether code is in the registry.
func Known(code string) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}

// CodeForStatus maps an HTTP status to its default code. Handlers that want a
// finer discriminator (two different 409s, say) pass a code explicitly; this is
// the floor so that no error path can ship without one.
func CodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return CodeBadRequest
	case http.StatusUnauthorized:
		return CodeUnauthorized
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusMethodNotAllowed:
		return CodeMethodNotAllowed
	case http.StatusConflict:
		return CodeConflict
	case http.StatusUnsupportedMediaType:
		return CodeUnsupportedMedia
	case http.StatusUnprocessableEntity:
		return CodeUnprocessableEntity
	case http.StatusTooManyRequests:
		return CodeRateLimitExceeded
	case http.StatusNotImplemented:
		return CodeNotImplemented
	case http.StatusServiceUnavailable:
		return CodeServiceUnavailable
	case http.StatusGatewayTimeout:
		return CodeGatewayTimeout
	default:
		if status >= 500 {
			return CodeInternalError
		}
		return CodeBadRequest
	}
}

// Error is the one error body octarq returns. Every field is part of the
// published contract.
type Error struct {
	// Code is the stable, machine-readable discriminator. Branch on this.
	Code string `json:"code" doc:"Stable machine-readable error code. Branch on this, never on message." example:"rate_limit_exceeded"`
	// Message is human-readable and may change at any time without notice.
	Message string `json:"message" doc:"Human-readable explanation. Not stable — never match on it." example:"1000 req/hr exceeded; retry after 30s"`
	// Details carries field-level context so the caller can self-diagnose,
	// typically one entry per failed field on a validation error.
	Details []*huma.ErrorDetail `json:"details,omitempty" doc:"Field-level context for self-diagnosis (validation errors carry one entry per bad field)."`
	// RequestID echoes the X-Request-Id of the failing request. Quoting it to
	// support is what makes a report traceable on our side.
	RequestID string `json:"request_id,omitempty" doc:"Request correlation ID; quote this to support." example:"a1b2c3d4"`

	status int
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Message }

// GetStatus implements huma.StatusError, so returning an *Error from a handler
// sets the response status.
func (e *Error) GetStatus() int { return e.status }

// Add implements huma.ErrorDetailer aggregation the same way huma.ErrorModel
// does, so huma's validation machinery can attach field details.
func (e *Error) Add(err error) {
	if converted, ok := err.(huma.ErrorDetailer); ok {
		e.Details = append(e.Details, converted.ErrorDetail())
		return
	}
	e.Details = append(e.Details, &huma.ErrorDetail{Message: err.Error()})
}

// New builds an envelope. status drives both the HTTP status and, when code is
// empty, the default code.
func New(status int, code, message string, errs ...error) *Error {
	if code == "" {
		code = CodeForStatus(status)
	}
	e := &Error{Code: code, Message: message, status: status}
	for _, err := range errs {
		if err == nil {
			continue
		}
		e.Add(err)
	}
	return e
}

// WithRequestID returns e with the request ID from ctx attached. It is a no-op
// when the context carries none (a request that never reached the edge
// middleware, e.g. in a unit test).
func (e *Error) WithRequestID(ctx context.Context) *Error {
	if e == nil || ctx == nil {
		return e
	}
	if id := server.RequestID(ctx); id != "" {
		e.RequestID = id
	}
	return e
}

// Write emits the envelope to a plain net/http response. This is the path for
// the layers that run outside huma — the CSRF guard, the plugin feature gate,
// panic recovery — so they produce the same body as every huma handler.
func Write(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	e := New(status, code, message)
	if r != nil {
		e.WithRequestID(r.Context())
	}
	// The edge middleware already echoed the ID into the response header; fall
	// back to it so a handler that only has the ResponseWriter still fills the
	// field.
	if e.RequestID == "" {
		e.RequestID = w.Header().Get("X-Request-Id")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
}

// JSON renders the envelope as a compact JSON body. It exists for the few call
// sites that must write bytes they already hold a status for (panic recovery
// inside a deferred func, where the ResponseWriter may be half-used).
func JSON(status int, code, message string) []byte {
	b, err := json.Marshal(New(status, code, message))
	if err != nil {
		return []byte(`{"code":"internal_error","message":"internal server error"}`)
	}
	return b
}

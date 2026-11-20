package apierror

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/server"
)

const (
	CodeBadRequest                = "bad_request"
	CodeValidationFailed          = "validation_failed"
	CodeUnauthorized              = "unauthorized"
	CodeForbidden                 = "forbidden"
	CodeCSRFOriginBlocked         = "csrf_origin_blocked"
	CodeCSRFTokenInvalid          = "csrf_token_invalid"
	CodeNotFound                  = "not_found"
	CodePluginNotEnabled          = "plugin_not_enabled"
	CodeMethodNotAllowed          = "method_not_allowed"
	CodeConflict                  = "conflict"
	CodeEmailInUse                = "email_in_use"
	CodeEmailVerificationRequired = "email_verification_required"
	CodeNoWorkspace               = "no_workspace"
	CodeWorkforceAgentNotFound    = "workforce_agent_not_found"
	CodeWorkforceAgentPaused      = "workforce_agent_paused"
	CodeWorkforceBudgetExceeded   = "workforce_budget_exceeded"
	CodeWorkforceModelPriceAbsent = "workforce_model_price_missing"
	CodeWorkforceSpendNotRecorded = "workforce_spend_not_recorded"
	CodeUnsupportedMedia          = "unsupported_media_type"
	CodeUnprocessableEntity       = "unprocessable_entity"
	CodeRateLimitExceeded         = "rate_limit_exceeded"
	CodeInternalError             = "internal_error"
	CodeNotImplemented            = "not_implemented"
	CodeServiceUnavailable        = "service_unavailable"
	CodeGatewayTimeout            = "gateway_timeout"
)

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
	CodeEmailInUse,
	CodeEmailVerificationRequired,
	CodeNoWorkspace,
	CodeWorkforceAgentNotFound,
	CodeWorkforceAgentPaused,
	CodeWorkforceBudgetExceeded,
	CodeWorkforceModelPriceAbsent,
	CodeWorkforceSpendNotRecorded,
	CodeUnsupportedMedia,
	CodeUnprocessableEntity,
	CodeRateLimitExceeded,
	CodeInternalError,
	CodeNotImplemented,
	CodeServiceUnavailable,
	CodeGatewayTimeout,
}

func Codes() []string {
	out := append([]string(nil), codes...)
	sort.Strings(out)
	return out
}

func Known(code string) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}

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

type Error struct {
	Code      string              `json:"code" doc:"Stable machine-readable error code. Branch on this, never on message." example:"rate_limit_exceeded"`
	Message   string              `json:"message" doc:"Human-readable explanation. Not stable — never match on it." example:"1000 req/hr exceeded; retry after 30s"`
	Details   []*huma.ErrorDetail `json:"details,omitempty" doc:"Field-level context for self-diagnosis (validation errors carry one entry per bad field)."`
	RequestID string              `json:"request_id,omitempty" doc:"Request correlation ID; quote this to support." example:"a1b2c3d4"`

	status int
}

func (e *Error) Error() string { return e.Message }

func (e *Error) GetStatus() int { return e.status }

func (e *Error) Add(err error) {
	if converted, ok := err.(huma.ErrorDetailer); ok {
		e.Details = append(e.Details, converted.ErrorDetail())
		return
	}
	e.Details = append(e.Details, &huma.ErrorDetail{Message: err.Error()})
}

func New(status int, code, message string, errs ...error) *Error {
	if code == "" {
		code = CodeForStatus(status)
	}
	e := &Error{Code: code, Message: message, status: status}
	for _, err := range errs {
		if err == nil {
			continue
		}
		if status >= 500 {
			slog.Error("internal error detail suppressed from response",
				"status", status, "message", message, "err", err)
			continue
		}
		e.Add(err)
	}
	return e
}

func (e *Error) WithRequestID(ctx context.Context) *Error {
	if e == nil || ctx == nil {
		return e
	}
	if id := server.RequestID(ctx); id != "" {
		e.RequestID = id
	}
	return e
}

func Write(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	e := New(status, code, message)
	if r != nil {
		e.WithRequestID(r.Context())
	}
	if e.RequestID == "" {
		e.RequestID = w.Header().Get("X-Request-Id")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
}

func JSON(status int, code, message string) []byte {
	b, err := json.Marshal(New(status, code, message))
	if err != nil {
		return []byte(`{"code":"internal_error","message":"internal server error"}`)
	}
	return b
}

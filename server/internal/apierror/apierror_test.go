package apierror

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/server"
)

func TestNewSuppresses5xxDetails(t *testing.T) {
	internalErr := errors.New("sql: connection pool exhausted; table users corrupted at /var/lib/data")

	// 500 error: internal details must not be in Details
	e500 := New(http.StatusInternalServerError, "", "database error", internalErr)
	if len(e500.Details) != 0 {
		t.Errorf("500 error must not leak details into Details slice, got: %+v", e500.Details)
	}

	b500, err := json.Marshal(e500)
	if err != nil {
		t.Fatal(err)
	}
	if string(b500) != `{"code":"internal_error","message":"database error"}` {
		t.Errorf("unexpected 500 json: %s", string(b500))
	}

	// 400 error: user error details must be preserved
	validationErr := errors.New("field 'email' is invalid")
	e400 := New(http.StatusBadRequest, "", "validation failed", validationErr)
	if len(e400.Details) != 1 {
		t.Fatalf("400 error must preserve detail, got length %d", len(e400.Details))
	}
	if e400.Details[0].Message != "field 'email' is invalid" {
		t.Errorf("unexpected detail message: %s", e400.Details[0].Message)
	}
}

func TestNew_NilErrorsAndCustomCode(t *testing.T) {
	e := New(http.StatusBadRequest, "custom_code", "test message", nil, errors.New("err1"), nil)
	if e.Code != "custom_code" {
		t.Errorf("expected code custom_code, got %s", e.Code)
	}
	if len(e.Details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(e.Details))
	}
}

func TestCodeForStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, CodeBadRequest},
		{http.StatusUnauthorized, CodeUnauthorized},
		{http.StatusForbidden, CodeForbidden},
		{http.StatusNotFound, CodeNotFound},
		{http.StatusMethodNotAllowed, CodeMethodNotAllowed},
		{http.StatusConflict, CodeConflict},
		{http.StatusUnsupportedMediaType, CodeUnsupportedMedia},
		{http.StatusUnprocessableEntity, CodeUnprocessableEntity},
		{http.StatusTooManyRequests, CodeRateLimitExceeded},
		{http.StatusInternalServerError, CodeInternalError},
		{http.StatusNotImplemented, CodeNotImplemented},
		{http.StatusBadGateway, CodeInternalError},
		{http.StatusServiceUnavailable, CodeServiceUnavailable},
		{http.StatusGatewayTimeout, CodeGatewayTimeout},
		{http.StatusTeapot, CodeBadRequest},
		{http.StatusHTTPVersionNotSupported, CodeInternalError},
	}
	for _, tc := range tests {
		if got := CodeForStatus(tc.status); got != tc.want {
			t.Errorf("CodeForStatus(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestCodes(t *testing.T) {
	got := Codes()
	if len(got) == 0 {
		t.Fatal("Codes() returned empty slice")
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("Codes() is not sorted: %v", got)
	}
	for _, code := range got {
		if !Known(code) {
			t.Errorf("Codes() returned unknown code: %q", code)
		}
	}
}

func TestKnown(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{CodeBadRequest, true},
		{CodeValidationFailed, true},
		{CodeUnauthorized, true},
		{CodeForbidden, true},
		{CodeCSRFOriginBlocked, true},
		{CodeCSRFTokenInvalid, true},
		{CodeNotFound, true},
		{CodePluginNotEnabled, true},
		{CodeMethodNotAllowed, true},
		{CodeConflict, true},
		{CodeUnsupportedMedia, true},
		{CodeUnprocessableEntity, true},
		{CodeRateLimitExceeded, true},
		{CodeInternalError, true},
		{CodeNotImplemented, true},
		{CodeServiceUnavailable, true},
		{CodeGatewayTimeout, true},
		{"random_code", false},
		{"", false},
	}

	for _, tc := range tests {
		if got := Known(tc.code); got != tc.want {
			t.Errorf("Known(%q) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestErrorMethods(t *testing.T) {
	e := New(http.StatusBadRequest, "", "bad request message")
	if e.GetStatus() != http.StatusBadRequest {
		t.Errorf("GetStatus() = %d, want %d", e.GetStatus(), http.StatusBadRequest)
	}
	if e.Error() != "bad request message" {
		t.Errorf("Error() = %q, want %q", e.Error(), "bad request message")
	}
}

type mockDetailer struct {
	msg string
}

func (m *mockDetailer) Error() string { return m.msg }
func (m *mockDetailer) ErrorDetail() *huma.ErrorDetail {
	return &huma.ErrorDetail{Message: m.msg}
}

func TestAdd(t *testing.T) {
	e := &Error{}
	e.Add(errors.New("test error"))
	if len(e.Details) != 1 {
		t.Fatalf("expected 1 error in slice, got %d", len(e.Details))
	}
	if e.Details[0].Message != "test error" {
		t.Errorf("expected 'test error', got %q", e.Details[0].Message)
	}

	// Test huma.ErrorDetailer implementation
	detailErr := &mockDetailer{msg: "detail message"}
	e.Add(detailErr)
	if len(e.Details) != 2 {
		t.Fatalf("expected 2 errors in slice, got %d", len(e.Details))
	}
	if e.Details[1].Message != "detail message" {
		t.Errorf("expected 'detail message', got %q", e.Details[1].Message)
	}
}

func TestWithRequestID(t *testing.T) {
	var nilErr *Error
	if got := nilErr.WithRequestID(context.Background()); got != nil {
		t.Errorf("expected nil when called on nil Error, got %v", got)
	}

	var nilCtx context.Context
	e := New(http.StatusBadRequest, "", "test")
	if got := e.WithRequestID(nilCtx); got != e {
		t.Errorf("expected original error when ctx is nil, got %v", got)
	}

	// Context without request ID
	eNoID := New(http.StatusBadRequest, "", "test").WithRequestID(context.Background())
	if eNoID.RequestID != "" {
		t.Errorf("expected empty request ID, got %q", eNoID.RequestID)
	}

	// Context with request ID
	ctx := context.WithValue(context.Background(), server.RequestIDKey, "req-12345")
	eWithID := New(http.StatusBadRequest, "", "test").WithRequestID(ctx)
	if eWithID.RequestID != "req-12345" {
		t.Errorf("expected request ID 'req-12345', got %q", eWithID.RequestID)
	}
}

func TestWrite(t *testing.T) {
	t.Run("with request context ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(r.Context(), server.RequestIDKey, "req-from-ctx")
		r = r.WithContext(ctx)

		Write(w, r, http.StatusNotFound, CodeNotFound, "not found")

		res := w.Result()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, res.StatusCode)
		}
		if got := res.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", got)
		}

		var e Error
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}
		if e.Code != CodeNotFound {
			t.Errorf("expected code %q, got %q", CodeNotFound, e.Code)
		}
		if e.Message != "not found" {
			t.Errorf("expected message %q, got %q", "not found", e.Message)
		}
		if e.RequestID != "req-from-ctx" {
			t.Errorf("expected request_id 'req-from-ctx', got %q", e.RequestID)
		}
	})

	t.Run("fallback to response header X-Request-Id", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Header().Set("X-Request-Id", "header-req-id")
		Write(w, nil, http.StatusBadRequest, CodeBadRequest, "bad request")

		var e Error
		if err := json.NewDecoder(w.Body).Decode(&e); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}
		if e.RequestID != "header-req-id" {
			t.Errorf("expected fallback request_id 'header-req-id', got %q", e.RequestID)
		}
	})
}

func TestJSON(t *testing.T) {
	got := JSON(http.StatusTeapot, "custom_error", "I'm a teapot")
	want := `{"code":"custom_error","message":"I'm a teapot"}`
	if string(got) != want {
		t.Errorf("JSON() = %q, want %q", string(got), want)
	}

	got = JSON(http.StatusNotFound, "", "Resource missing")
	want = `{"code":"not_found","message":"Resource missing"}`
	if string(got) != want {
		t.Errorf("JSON() = %q, want %q", string(got), want)
	}

	got = JSON(http.StatusInternalServerError, "", "fail")
	want = `{"code":"internal_error","message":"fail"}`
	if string(got) != want {
		t.Errorf("JSON() = %q, want %q", string(got), want)
	}
}

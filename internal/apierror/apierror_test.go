package apierror

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
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
		{http.StatusServiceUnavailable, CodeServiceUnavailable},
		{http.StatusGatewayTimeout, CodeGatewayTimeout},
		{http.StatusBadGateway, CodeInternalError},
		{http.StatusTeapot, CodeBadRequest},
	}
	for _, tc := range tests {
		if got := CodeForStatus(tc.status); got != tc.want {
			t.Errorf("CodeForStatus(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

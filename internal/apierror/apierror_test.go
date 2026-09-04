package apierror

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
		{http.StatusConflict, CodeConflict},
		{http.StatusInternalServerError, CodeInternalError},
		{http.StatusBadGateway, CodeInternalError},
	}
	for _, tc := range tests {
		if got := CodeForStatus(tc.status); got != tc.want {
			t.Errorf("CodeForStatus(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestWrite(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
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
}

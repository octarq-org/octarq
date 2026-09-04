package apierror

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/internal/server"
)

func TestCodesAndKnown(t *testing.T) {
	codes := Codes()
	if len(codes) == 0 {
		t.Fatal("Codes empty")
	}
	for _, c := range codes {
		if !Known(c) {
			t.Errorf("Known(%q) false", c)
		}
	}
	if Known("not_a_code") {
		t.Error("Known true for unknown")
	}
}

func TestErrorMethods(t *testing.T) {
	e := New(http.StatusBadRequest, "", "bad")
	if e.GetStatus() != http.StatusBadRequest {
		t.Error("GetStatus")
	}
	if e.Error() != "bad" {
		t.Error("Error()")
	}
	_ = e.WithRequestID(context.TODO())
	_ = e.WithRequestID(context.Background())
	ctx := context.WithValue(context.Background(), server.RequestIDKey, "req-123")
	e2 := New(http.StatusBadRequest, CodeBadRequest, "x").WithRequestID(ctx)
	if e2.RequestID != "req-123" {
		t.Errorf("WithRequestID %q", e2.RequestID)
	}
	e.Add(New(http.StatusBadRequest, CodeBadRequest, "detail"))
	if len(e.Details) == 0 {
		t.Error("Add should append details")
	}
}

func TestWriteAndJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	Write(w, r, http.StatusBadRequest, CodeBadRequest, "oops")
	if w.Code != http.StatusBadRequest {
		t.Error("Write status")
	}
	b := JSON(http.StatusNotFound, CodeNotFound, "missing")
	if len(b) == 0 || string(b) == "" {
		t.Error("JSON empty")
	}
	b2 := JSON(http.StatusInternalServerError, "", "fail")
	_ = b2
}

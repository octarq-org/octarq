package api

import (
	"testing"

	"github.com/octarq-org/octarq/internal/apierror"
)

func TestErrorCodes(t *testing.T) {
	codes := ErrorCodes()
	expected := apierror.Codes()

	if len(codes) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(codes))
	}

	for i := range expected {
		if codes[i] != expected[i] {
			t.Errorf("expected code at index %d to be %q, got %q", i, expected[i], codes[i])
		}
	}
}

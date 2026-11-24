package api

import (
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/apierror"
)

func TestNormalizeErrorStatusAiSlice(t *testing.T) {
	status, code := normalizeErrorStatus(http.StatusUnprocessableEntity, validationFailedMsg)
	if status != http.StatusBadRequest || code != apierror.CodeValidationFailed {
		t.Errorf("validation 422 = (%d, %q)", status, code)
	}
	status, _ = normalizeErrorStatus(http.StatusInternalServerError, "boom")
	if status != http.StatusInternalServerError {
		t.Errorf("500 = %d", status)
	}
}

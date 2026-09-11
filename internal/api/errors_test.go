package api

import (
	"reflect"
	"testing"

	"github.com/octarq-org/octarq/internal/apierror"
)

func TestErrorCodes(t *testing.T) {
	got := ErrorCodes()
	want := apierror.Codes()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ErrorCodes() = %v, want %v", got, want)
	}
}

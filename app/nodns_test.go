//go:build octarq_nodns

package app_test

import (
	"testing"

	"github.com/octarq-org/octarq/plugins/builtin"
)

func TestNodnsBuildTag(t *testing.T) {
	if builtin.DNS() != nil {
		t.Fatal("expected DNS plugin to be nil under octarq_nodns build tag")
	}
}

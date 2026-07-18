//go:build octarq_nolinks

package app_test

import (
	"testing"

	"github.com/octarq-org/octarq/plugins/builtin"
)

func TestNolinksBuildTag(t *testing.T) {
	if builtin.Links() != nil {
		t.Fatal("expected Links plugin to be nil under octarq_nolinks build tag")
	}
}

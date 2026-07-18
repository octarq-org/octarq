//go:build octarq_nomail

package app_test

import (
	"testing"

	"github.com/octarq-org/octarq/plugins/builtin"
)

func TestNomailBuildTag(t *testing.T) {
	if builtin.Mail() != nil {
		t.Fatal("expected Mail plugin to be nil under octarq_nomail build tag")
	}
}

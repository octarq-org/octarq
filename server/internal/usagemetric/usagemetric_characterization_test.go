package usagemetric_test

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/usagemetric"
)

func TestUsageMetricCharacterizationConstants(t *testing.T) {
	metrics := map[string]string{
		"Clicks":   usagemetric.Clicks,
		"MailOut":  usagemetric.MailOut,
		"MailIn":   usagemetric.MailIn,
		"RawBytes": usagemetric.RawBytes,
	}

	expected := map[string]string{
		"Clicks":   "clicks",
		"MailOut":  "mailOut",
		"MailIn":   "mailIn",
		"RawBytes": "mail.raw_bytes",
	}

	for k, exp := range expected {
		if metrics[k] != exp {
			t.Fatalf("expected %s to be %q, got %q", k, exp, metrics[k])
		}
	}
}

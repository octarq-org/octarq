package api

import (
	"os"
	"testing"

	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
	"golang.org/x/crypto/bcrypt"
)

// TestMain relaxes the webhook SSRF guard for the test binary so tests that
// deliver to loopback httptest servers (e.g. notification-channel tests) work.
// Production keeps the guard strict unless the operator opts in via
// OCTARQ_ALLOW_PRIVATE_WEBHOOKS.
func TestMain(m *testing.M) {
	safehttp.SetAllowPrivateWebhooks(true)
	auth.BcryptCost = bcrypt.MinCost
	initTestDBTemplates()
	os.Exit(m.Run())
}

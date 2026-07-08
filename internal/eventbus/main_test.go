package eventbus

import (
	"os"
	"testing"

	"github.com/octarq-org/octarq/internal/safehttp"
)

// TestMain relaxes the webhook SSRF guard for the test binary so deliver() can
// reach loopback httptest servers. Production keeps the guard strict unless the
// operator opts in via OCTARQ_ALLOW_PRIVATE_WEBHOOKS.
func TestMain(m *testing.M) {
	safehttp.SetAllowPrivateWebhooks(true)
	os.Exit(m.Run())
}

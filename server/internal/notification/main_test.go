package notification

import (
	"os"
	"testing"

	"github.com/octarq-org/octarq/server/plugin/safehttp"
)

func TestMain(m *testing.M) {
	safehttp.SetAllowPrivateWebhooks(true)
	os.Exit(m.Run())
}

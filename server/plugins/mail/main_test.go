package mail

import (
	"os"
	"testing"

	"github.com/octarq-org/octarq/server/plugin/safehttp"
)

func TestMain(m *testing.M) {
	safehttp.SetAllowPrivateSMTP(true)
	os.Exit(m.Run())
}

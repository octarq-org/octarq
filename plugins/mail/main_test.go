package mail

import (
	"os"
	"testing"

	"github.com/octarq-org/octarq/internal/safehttp"
)

func TestMain(m *testing.M) {
	safehttp.SetAllowPrivateSMTP(true)
	os.Exit(m.Run())
}

package api

import (
	"context"
	"net/http"
	"time"

	"github.com/octarq-org/octarq/plugin/safehttp"
)

// safePreviewClient is the shared client for fetching user-supplied URLs. It
// blocks non-public destinations (incl. across redirects), caps redirects, and
// has tight timeouts so a slow or huge target can't tie up the server.
var safePreviewClient = safehttp.NewClient(10 * time.Second)

// safeGet issues a guarded GET for a user-supplied URL.
func safeGet(ctx context.Context, rawURL string) (*http.Response, error) {
	return safehttp.Get(ctx, safePreviewClient, rawURL, "Mozilla/5.0 (compatible; octarq-link-preview/1.0)")
}

package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/octarq-org/octarq/internal/safehttp"
)

// safePreviewClient is the shared client for fetching user-supplied URLs. It
// blocks non-public destinations (incl. across redirects), caps redirects, and
// has tight timeouts so a slow or huge target can't tie up the server.
var safePreviewClient = safehttp.NewClient(10 * time.Second)

// safeGet issues a guarded GET for a user-supplied URL. It rejects any scheme
// other than http/https before dialing (so file://, gopher://, etc. never run).
func safeGet(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, fmt.Errorf("ssrf guard: disallowed scheme %q", req.URL.Scheme)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; octarq-link-preview/1.0)")
	return safePreviewClient.Do(req)
}

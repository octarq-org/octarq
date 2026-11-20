package api

import (
	"context"
	"net/http"
	"time"

	"github.com/octarq-org/octarq/server/plugin/safehttp"
)

var safePreviewClient = safehttp.NewClient(10 * time.Second)

func safeGet(ctx context.Context, rawURL string) (*http.Response, error) {
	return safehttp.Get(ctx, safePreviewClient, rawURL, "Mozilla/5.0 (compatible; octarq-link-preview/1.0)")
}

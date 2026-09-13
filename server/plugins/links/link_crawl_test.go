package links

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchPageMetaBlocksInternal(t *testing.T) {
	// End-to-end: the title-preview path must return nothing for an internal URL
	// rather than fetching it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<title>internal</title>"))
	}))
	defer srv.Close()

	title, desc := fetchPageMeta(context.Background(), srv.URL)
	if title != "" || desc != "" {
		t.Errorf("fetchPageMeta fetched an internal URL: title=%q desc=%q", title, desc)
	}
}

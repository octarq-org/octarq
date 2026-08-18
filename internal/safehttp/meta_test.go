package safehttp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchPageMeta_Parsing(t *testing.T) {
	SetAllowPrivateWebhooks(true)
	defer SetAllowPrivateWebhooks(false)

	oldClient := metaClient
	t.Cleanup(func() { metaClient = oldClient })
	metaClient = NewWebhookClient(5 * time.Second)

	tests := []struct {
		name  string
		html  string
		wantT string
		wantD string
	}{
		{
			name:  "og:title and description",
			html:  `<html><head><meta property="og:title" content="My &amp; OG Title"><meta name="description" content="My &quot;Description&quot;"></head><body></body></html>`,
			wantT: `My & OG Title`,
			wantD: `My "Description"`,
		},
		{
			name:  "og:title with content first",
			html:  `<html><head><meta content="Inverted OG Title" property="og:title"><meta name='description' content='Single quoted desc'></head></html>`,
			wantT: `Inverted OG Title`,
			wantD: `Single quoted desc`,
		},
		{
			name:  "standard title fallback",
			html:  `<html><head><title>  Regular Title &lt;3  </title></head></html>`,
			wantT: `Regular Title <3`,
			wantD: ``,
		},
		{
			name:  "empty html",
			html:  `<html><body>No meta here</body></html>`,
			wantT: ``,
			wantD: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, tt.html)
			}))
			defer ts.Close()

			gotT, gotD := FetchPageMeta(context.Background(), ts.URL)
			if gotT != tt.wantT {
				t.Errorf("title: got %q, want %q", gotT, tt.wantT)
			}
			if gotD != tt.wantD {
				t.Errorf("desc: got %q, want %q", gotD, tt.wantD)
			}
		})
	}
}

func TestFetchPageMeta_InvalidURL(t *testing.T) {
	title, desc := FetchPageMeta(context.Background(), "file:///etc/hosts")
	if title != "" || desc != "" {
		t.Errorf("expected empty result for invalid scheme URL, got %q, %q", title, desc)
	}
}

func TestGet_InvalidSchemeAndBadURL(t *testing.T) {
	client := NewClient(5 * time.Second)
	_, err := Get(context.Background(), client, "ftp://example.com", "UA")
	if err == nil {
		t.Error("expected error for ftp scheme, got nil")
	}

	_, err = Get(context.Background(), client, "http://\x7f/bad", "UA")
	if err == nil {
		t.Error("expected error for malformed URL, got nil")
	}
}

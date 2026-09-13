package links

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin/safehttp"
)

var (
	reTitle           = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reDesc            = regexp.MustCompile(`(?is)<meta[^>]+name=["']description["'][^>]+content=["'](.*?)["']`)
	reOgTitle         = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["'](.*?)["']`)
	reOgTitle2        = regexp.MustCompile(`(?is)<meta[^>]+content=["'](.*?)["'][^>]+property=["']og:title["']`)
	safePreviewClient = safehttp.NewClient(10 * time.Second)
)

func safeGet(ctx context.Context, rawURL string) (*http.Response, error) {
	return safehttp.Get(ctx, safePreviewClient, rawURL, "Mozilla/5.0 (compatible; octarq-link-preview/1.0)")
}

func fetchPageMeta(ctx context.Context, rawURL string) (title, desc string) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	resp, err := safeGet(ctx, rawURL)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	return parsePageMeta(body)
}

// parsePageMeta is split from fetchPageMeta so extraction is testable without
// a network round-trip (the SSRF-guarded client refuses loopback by design).
func parsePageMeta(body []byte) (title, desc string) {
	if m := reOgTitle.FindSubmatch(body); m != nil {
		title = strings.TrimSpace(html.UnescapeString(string(m[1])))
	} else if m := reOgTitle2.FindSubmatch(body); m != nil {
		title = strings.TrimSpace(html.UnescapeString(string(m[1])))
	} else if m := reTitle.FindSubmatch(body); m != nil {
		title = strings.TrimSpace(html.UnescapeString(string(m[1])))
	}
	if m := reDesc.FindSubmatch(body); m != nil {
		desc = strings.TrimSpace(html.UnescapeString(string(m[1])))
	}
	return title, desc
}

type LinkMetadataInput struct {
	Ctx huma.Context `hidden:"true"`
	URL string       `query:"url"`
}

func (i *LinkMetadataInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LinkMetadataOutput struct {
	Body map[string]any
}

// linkMetadata fetches the target page's <title>, description, and favicon so
// the dashboard can prefill a link's title (dub-style). Best-effort.
func (p *Plugin) linkMetadata(ctx context.Context, input *LinkMetadataInput) (*LinkMetadataOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	raw := strings.TrimSpace(input.URL)
	if raw == "" {
		return nil, huma.Error400BadRequest("url required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, huma.Error400BadRequest("invalid url")
	}
	title, desc := fetchPageMeta(r.Context(), raw)
	favicon := u.Scheme + "://" + u.Host + "/favicon.ico"
	return &LinkMetadataOutput{
		Body: map[string]any{
			"title": title, "description": desc, "favicon": favicon,
		},
	}, nil
}

func (p *Plugin) handleLinkCrawl(ctx context.Context, payload []byte) error {
	var d struct {
		ID     uint   `json:"id"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal(payload, &d); err != nil {
		return err
	}
	title, _ := fetchPageMeta(ctx, d.Target)
	if title != "" {
		return p.db.Model(&Link{}).Where("id = ?", d.ID).Update("title", title).Error
	}
	return nil
}

package links

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/safehttp"
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

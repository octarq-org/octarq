package api

import (
	"context"
	"html"
	"io"
	"regexp"
	"strings"
	"time"
)

var (
	reTitle    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reDesc     = regexp.MustCompile(`(?is)<meta[^>]+name=["']description["'][^>]+content=["'](.*?)["']`)
	reOgTitle  = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["'](.*?)["']`)
	reOgTitle2 = regexp.MustCompile(`(?is)<meta[^>]+content=["'](.*?)["'][^>]+property=["']og:title["']`)
)

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

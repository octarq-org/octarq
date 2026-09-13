package links

import (
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/octarq-org/octarq/internal/models"
	"golang.org/x/net/publicsuffix"
)

// classifyReferer categorizes a raw referer string into a channel name.
func classifyReferer(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "Direct"
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if !strings.Contains(raw, "://") {
			u, err = url.Parse("http://" + raw)
		}
	}
	if err != nil || u == nil || u.Host == "" {
		return "Direct"
	}

	host := u.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "Direct"
	}

	if host == "t.co" || host == "twitter.com" || strings.HasSuffix(host, ".twitter.com") || host == "x.com" || strings.HasSuffix(host, ".x.com") {
		return "Twitter"
	}
	if host == "google.com" || strings.HasSuffix(host, ".google.com") || strings.HasPrefix(host, "google.") || strings.Contains(host, ".google.") {
		return "Google"
	}
	if host == "facebook.com" || strings.HasSuffix(host, ".facebook.com") || host == "fb.com" || host == "fb.me" || host == "instagram.com" || strings.HasSuffix(host, ".instagram.com") {
		return "Facebook"
	}
	if host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com") || host == "lnkd.in" {
		return "LinkedIn"
	}
	if host == "reddit.com" || strings.HasSuffix(host, ".reddit.com") || host == "redd.it" {
		return "Reddit"
	}
	if host == "news.ycombinator.com" || host == "ycombinator.com" || strings.HasSuffix(host, ".ycombinator.com") {
		return "Hacker News"
	}
	if host == "github.com" || strings.HasSuffix(host, ".github.com") {
		return "GitHub"
	}
	if host == "weixin.qq.com" || host == "mp.weixin.qq.com" || strings.HasSuffix(host, ".weixin.qq.com") || host == "qq.com" || strings.HasSuffix(host, ".qq.com") {
		return "WeChat"
	}
	if host == "zhihu.com" || strings.HasSuffix(host, ".zhihu.com") {
		return "Zhihu"
	}

	if domain, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil && domain != "" {
		return domain
	}

	return host
}

func sortStatKV(res []models.StatKV) {
	sort.Slice(res, func(i, j int) bool {
		if res[i].Count != res[j].Count {
			return res[i].Count > res[j].Count
		}
		return res[i].Key < res[j].Key
	})
}

// shortURL builds the public URL for a link. When the link has its own host it
// is used; otherwise the URL is derived from the incoming request so no extra
// configuration is needed.
func shortURL(r *http.Request, l Link) string {
	host := l.Host
	scheme := "https"
	if host == "" {
		host = r.Host
		if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
			scheme = "http"
		}
	}
	return scheme + "://" + host + "/" + l.Slug
}

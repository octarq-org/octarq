package links

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/net/publicsuffix"
)

type LinkStatsInput struct {
	Ctx    huma.Context `hidden:"true"`
	ID     uint         `path:"id"`
	Days   int          `query:"days"`
	Metric string       `query:"metric"`
}

func (i *LinkStatsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LinkStatsOutput struct {
	Body map[string]any
}

// linkStats returns basic analytics: totals, a daily time series, and the
// top referers / countries / devices / browsers over the requested window.
func (p *Plugin) linkStats(ctx context.Context, input *LinkStatsInput) (*LinkStatsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	// Ensure the link belongs to the caller's org before exposing its analytics.
	var l Link
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&l).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	days := 30
	if input.Days > 0 && input.Days <= 365 {
		days = input.Days
	}
	since := time.Now().AddDate(0, 0, -days)

	metric := strings.ToLower(strings.TrimSpace(input.Metric))
	if metric != "pv" {
		metric = "uv"
	}
	isUV := metric == "uv"

	top := func(col string) []models.StatKV {
		rows := make([]models.StatKV, 0)
		q := p.db.Model(&LinkEvent{}).
			Where("link_id = ? AND created_at >= ? AND "+col+" <> ''", input.ID, since)
		if isUV {
			q = q.Where("fingerprint <> ''").
				Select(col + " as key, count(distinct fingerprint) as count")
		} else {
			q = q.Select(col + " as key, count(*) as count")
		}
		q.Group(col).Order("count DESC").Limit(10).Scan(&rows)
		return rows
	}

	var total int64
	p.db.Model(&LinkEvent{}).Where("link_id = ?", input.ID).Count(&total)

	series := make([]models.StatKV, 0)
	p.db.Model(&LinkEvent{}).
		Select("strftime('%Y-%m-%d', created_at) as key, count(*) as count").
		Where("link_id = ? AND created_at >= ?", input.ID, since).
		Group("key").Order("key ASC").Scan(&series)
	// Postgres uses to_char; fall back when sqlite strftime yields nothing.
	if len(series) == 0 && p.db.Name() == "postgres" {
		p.db.Model(&LinkEvent{}).
			Select("to_char(created_at, 'YYYY-MM-DD') as key, count(*) as count").
			Where("link_id = ? AND created_at >= ?", input.ID, since).
			Group("key").Order("key ASC").Scan(&series)
	}

	var variants []models.StatKV
	for _, rule := range l.RoutingRules {
		if rule.Type == "split" {
			variants = top("variant")
			break
		}
	}

	return &LinkStatsOutput{
		Body: map[string]any{
			"total":        total,
			"windowed":     models.SumStatKV(series),
			"days":         days,
			"metric":       metric,
			"series":       series,
			"referers":     top("referer"),
			"channels":     p.topChannels(input.ID, since, isUV),
			"countries":    top("country"),
			"regions":      top("region"),
			"devices":      top("device"),
			"browsers":     top("browser"),
			"utmSources":   top("utm_source"),
			"utmMediums":   top("utm_medium"),
			"utmCampaigns": top("utm_campaign"),
			"variants":     variants,
		},
	}, nil
}

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

func (p *Plugin) topChannels(linkID uint, since time.Time, isUV bool) []models.StatKV {
	type row struct {
		Referer     string
		Fingerprint string
	}
	var rows []row
	q := p.db.Model(&LinkEvent{}).
		Where("link_id = ? AND created_at >= ? AND referer <> ''", linkID, since)
	if isUV {
		q = q.Where("fingerprint <> ''")
	}
	q.Select("referer, fingerprint").Scan(&rows)

	if len(rows) == 0 {
		return []models.StatKV{}
	}

	if isUV {
		channelFP := make(map[string]map[string]struct{})
		for _, r := range rows {
			ch := classifyReferer(r.Referer)
			if channelFP[ch] == nil {
				channelFP[ch] = make(map[string]struct{})
			}
			channelFP[ch][r.Fingerprint] = struct{}{}
		}
		res := make([]models.StatKV, 0, len(channelFP))
		for ch, fps := range channelFP {
			res = append(res, models.StatKV{Key: ch, Count: int64(len(fps))})
		}
		sortStatKV(res)
		if len(res) > 10 {
			res = res[:10]
		}
		return res
	} else {
		counts := make(map[string]int64)
		for _, r := range rows {
			ch := classifyReferer(r.Referer)
			counts[ch]++
		}
		res := make([]models.StatKV, 0, len(counts))
		for ch, cnt := range counts {
			res = append(res, models.StatKV{Key: ch, Count: cnt})
		}
		sortStatKV(res)
		if len(res) > 10 {
			res = res[:10]
		}
		return res
	}
}

func sortStatKV(res []models.StatKV) {
	sort.Slice(res, func(i, j int) bool {
		if res[i].Count != res[j].Count {
			return res[i].Count > res[j].Count
		}
		return res[i].Key < res[j].Key
	})
}

type LinkQRInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *LinkQRInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (p *Plugin) linkQR(ctx context.Context, input *LinkQRInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var l Link
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&l).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	target := shortURL(r, l)
	png, err := qrcode.Encode(target, qrcode.Medium, 320)
	if err != nil {
		return nil, huma.Error500InternalServerError("qr failed")
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
	return nil, nil
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

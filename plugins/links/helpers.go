package links

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// normalizeTarget trims a user-supplied redirect target, defaults a bare host
// to https, and rejects anything that isn't a well-formed http(s) URL. This
// keeps javascript:, data:, and other dangerous schemes out of a stored link
// (which is later emitted verbatim in a 302 Location header). Returns the
// normalized URL and true on success, or ("", false) when it must be refused.
func normalizeTarget(raw string) (string, bool) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return raw, true
}

// validateRedirectTargets checks every user-supplied redirect target on a link
// that is later emitted verbatim in a 302 Location header — the ExpiredURL and
// each RoutingRule.Target — against normalizeTarget's http(s) scheme allowlist.
// Unlike the primary Target these are not normalized in place (they may be
// stored raw), so this rejects javascript:, data:, etc. at write time. An empty
// ExpiredURL is allowed (it just means "404 when expired"). It normalizes the
// accepted values in place so a bare host defaults to https like Target does.
func validateRedirectTargets(l *Link) error {
	if l.ExpiredURL != "" {
		n, ok := normalizeTarget(strings.TrimSpace(l.ExpiredURL))
		if !ok {
			return huma.Error400BadRequest("expiredUrl must be an http(s) URL")
		}
		l.ExpiredURL = n
	}
	for i := range l.RoutingRules {
		n, ok := normalizeTarget(strings.TrimSpace(l.RoutingRules[i].Target))
		if !ok {
			return huma.Error400BadRequest("routing rule target must be an http(s) URL")
		}
		l.RoutingRules[i].Target = n
	}
	return nil
}

func quotaErrorToHTTP(err error, metric string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, plugin.ErrQuotaUnavailable) {
		return huma.Error402PaymentRequired(metric + " is not included in this plan")
	}
	return huma.Error429TooManyRequests(metric + " quota exceeded for this workspace")
}

// checkQuota asks the (hosted-only) quota checker whether the org may consume
// n more of a metered resource, and maps a refusal to the HTTP error a client
// should see. An exhausted allowance is a 429; a capability the plan simply
// does not include is a 402 upgrade prompt — the two must stay distinct
// because the dashboard renders them differently. With no checker registered
// (self-hosted) it always passes: unlimited links are the self-hosted selling
// point.
func (p *Plugin) checkQuota(ctx context.Context, orgID uint, metric string, n int64) error {
	if err := plugin.CheckQuota(p.ctx, ctx, orgID, metric, n); err != nil {
		return quotaErrorToHTTP(err, metric)
	}
	return nil
}

// escapeLike escapes LIKE metacharacters so a user tag of `_` or `100%`
// cannot match extra rows. '!' is the ESCAPE character (portable, unlike '\').
func escapeLike(s string) string {
	r := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return r.Replace(s)
}

// filterByTag is the SQL counterpart of tagsContain: case-insensitive, comma
// token match, surrounding spaces ignored. Spaces are stripped rather than
// enumerated as LIKE arms so Postgres (case-sensitive LIKE) and ", b ,"
// whitespace stay in lockstep with the Go helper.
func filterByTag(q *gorm.DB, tag string) *gorm.DB {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return q
	}
	folded := strings.ToLower(strings.ReplaceAll(tag, " ", ""))
	esc := escapeLike(folded)
	norm := "LOWER(REPLACE(tags, ' ', ''))"
	return q.Where(
		norm+" = ? OR "+norm+" LIKE ? ESCAPE '!' OR "+norm+" LIKE ? ESCAPE '!' OR "+norm+" LIKE ? ESCAPE '!'",
		folded,
		esc+",%",
		"%,"+esc,
		"%,"+esc+",%",
	)
}

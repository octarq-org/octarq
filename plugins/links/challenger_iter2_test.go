package links

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
)

// patchedCreateDeclarativeLink simulates the proposed remediation in AUDIT_REPORT.md (Finding SEC-02)
func (p *Plugin) patchedCreateDeclarativeLink(ctx context.Context, in DeclarativeLinkInput, auditCapture *[]string, eventCapture *[]string) (*DeclarativeLinkOutput, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, plugin.NewAgentError(401, "UNAUTHORIZED", "unauthorized: missing workspace", "Ensure an authenticated session or API token is provided.", false)
	}
	if p.linkHostRequired(orgID) {
		return nil, plugin.NewAgentError(400, "HOST_REQUIRED", "host is required in multi-tenant mode", "Please configure a custom link host first.", false)
	}
	if err := p.checkQuota(ctx, orgID, "links", 1); err != nil {
		code := 429
		errCode := "QUOTA_EXCEEDED"
		if se, ok := err.(huma.StatusError); ok && se.GetStatus() == http.StatusPaymentRequired {
			code = 402
			errCode = "FEATURE_UNAVAILABLE"
		}
		return nil, plugin.NewAgentError(code, errCode, err.Error(), "Upgrade plan to create more links.", false)
	}
	dest := strings.TrimSpace(in.Destination)
	if dest == "" {
		return nil, plugin.NewAgentError(400, "MISSING_DESTINATION", "destination is required", "Please provide a valid destination URL.", false)
	}
	normalized, ok := normalizeTarget(dest)
	if !ok {
		return nil, plugin.NewAgentError(400, "INVALID_DESTINATION", "destination must be an http(s) URL", "Please provide a valid URL starting with http:// or https://.", false)
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = models.RandomSlug(6)
	}
	if p.isReservedSlug(slug) {
		return nil, plugin.NewAgentError(409, "SLUG_RESERVED", "slug is reserved", "The specified slug is a system reserved path. Please pick a different slug.", false)
	}
	tagStr := strings.Join(in.Tags, ",")
	l := Link{
		OrgID:     orgID,
		Slug:      slug,
		Target:    normalized,
		Tags:      tagStr,
		Enabled:   true,
		CreatedAt: time.Now(),
	}
	if err := p.db.WithContext(ctx).Create(&l).Error; err != nil {
		return nil, plugin.NewAgentError(409, "SLUG_ALREADY_EXISTS", "slug already exists on this host", "The slug is already taken. Please choose another slug or leave it blank.", false)
	}
	if p.audit != nil {
		p.audit(nil, "link.create", "link", l.ID, map[string]any{"slug": l.Slug, "target": l.Target, "source": "declarative"})
		if auditCapture != nil {
			*auditCapture = append(*auditCapture, fmt.Sprintf("audit:link.create:%d:%s", l.ID, l.Slug))
		}
	}
	if p.publishEvent != nil {
		p.publishEvent(l.OrgID, "link.create", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host, "target": l.Target})
		if eventCapture != nil {
			*eventCapture = append(*eventCapture, fmt.Sprintf("event:link.create:%d:%s", l.OrgID, l.Slug))
		}
	}
	if p.deleteCache != nil {
		_ = p.deleteCache(ctx, "link:redirect:"+l.Host+":"+l.Slug)
	}
	return &DeclarativeLinkOutput{
		ID:          l.ID,
		Slug:        l.Slug,
		Destination: l.Target,
		CreatedAt:   l.CreatedAt,
	}, nil
}

// TestVerification_SEC02_Remediation verifies the refined SEC-02 remediation in AUDIT_REPORT.md
func TestVerification_SEC02_Remediation(t *testing.T) {
	t.Parallel()
	p, _ := setupFullLinksTestDB(t)
	ctxOrg1 := plugin.WithOrgID(context.Background(), 1)

	var auditLogs []string
	var eventLogs []string
	p.audit = func(_ *http.Request, action, resource string, resourceID uint, extra map[string]any) {
		auditLogs = append(auditLogs, fmt.Sprintf("%s:%s:%d:source=%v", action, resource, resourceID, extra["source"]))
	}
	p.publishEvent = func(orgID uint, event string, payload any) {
		eventLogs = append(eventLogs, fmt.Sprintf("%d:%s", orgID, event))
	}

	// 1. Test Host Requirement Enforcement
	p.db.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "octarq.test"})
	p.db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "mybrand.com",
		ForLink: true,
		LinkHosts: models.HostList{
			{Host: "go.mybrand.com", Enabled: true},
		},
	})

	if !p.linkHostRequired(1) {
		t.Fatal("expected linkHostRequired to be true")
	}

	// Calling patchedCreateDeclarativeLink must reject with 400 HOST_REQUIRED
	_, errHost := p.patchedCreateDeclarativeLink(ctxOrg1, DeclarativeLinkInput{
		Slug:        "test-host-req",
		Destination: "https://example.com",
	}, nil, nil)
	if errHost == nil {
		t.Fatal("expected error on host required, got nil")
	}
	var agentErr *plugin.AgentError
	if errors.As(errHost, &agentErr) {
		if agentErr.HTTPCode != 400 || agentErr.Code != "HOST_REQUIRED" {
			t.Errorf("expected 400 HOST_REQUIRED, got %d %s", agentErr.HTTPCode, agentErr.Code)
		}
	} else {
		t.Fatalf("expected *plugin.AgentError, got %T: %v", errHost, errHost)
	}

	// Reset host requirement to test quota paths
	p.db.Where("key = ?", models.BaseDomainSetting).Delete(&models.Setting{})
	if p.linkHostRequired(1) {
		t.Fatal("expected linkHostRequired to be false after removing base domain setting")
	}

	// 2. Test Quota 402 Payment Required (Feature unavailable in tier)
	withQuotaChecker(p, fakeQuotaChecker{err: plugin.ErrQuotaUnavailable})
	_, err402 := p.patchedCreateDeclarativeLink(ctxOrg1, DeclarativeLinkInput{
		Slug:        "test-402",
		Destination: "https://example.com",
	}, nil, nil)
	if err402 == nil {
		t.Fatal("expected error for 402 quota check")
	}
	if errors.As(err402, &agentErr) {
		if agentErr.HTTPCode != 402 || agentErr.Code != "FEATURE_UNAVAILABLE" {
			t.Errorf("expected 402 FEATURE_UNAVAILABLE for payment required, got %d %s", agentErr.HTTPCode, agentErr.Code)
		}
	} else {
		t.Fatalf("expected *plugin.AgentError, got %T: %v", err402, err402)
	}

	// 3. Test Quota 429 Too Many Requests (Exceeded link count quota)
	withQuotaChecker(p, fakeQuotaChecker{err: plugin.ErrQuotaExceeded})
	_, err429 := p.patchedCreateDeclarativeLink(ctxOrg1, DeclarativeLinkInput{
		Slug:        "test-429",
		Destination: "https://example.com",
	}, nil, nil)
	if err429 == nil {
		t.Fatal("expected error for 429 quota check")
	}
	if errors.As(err429, &agentErr) {
		if agentErr.HTTPCode != 429 || agentErr.Code != "QUOTA_EXCEEDED" {
			t.Errorf("expected 429 QUOTA_EXCEEDED for rate limit, got %d %s", agentErr.HTTPCode, agentErr.Code)
		}
	} else {
		t.Fatalf("expected *plugin.AgentError, got %T: %v", err429, err429)
	}

	// 4. Test Generic Quota error fallback (defaults to 429 QUOTA_EXCEEDED)
	withQuotaChecker(p, fakeQuotaChecker{err: errors.New("generic quota error")})
	_, errGen := p.patchedCreateDeclarativeLink(ctxOrg1, DeclarativeLinkInput{
		Slug:        "test-gen-quota",
		Destination: "https://example.com",
	}, nil, nil)
	if errGen == nil {
		t.Fatal("expected error for generic quota check")
	}
	if errors.As(errGen, &agentErr) {
		if agentErr.HTTPCode != 429 || agentErr.Code != "QUOTA_EXCEEDED" {
			t.Errorf("expected 429 QUOTA_EXCEEDED for generic quota error, got %d %s", agentErr.HTTPCode, agentErr.Code)
		}
	}

	// 5. Test Successful Creation with Audit and Event Publishing
	withQuotaChecker(p, fakeQuotaChecker{err: nil})
	outSuccess, errSuccess := p.patchedCreateDeclarativeLink(ctxOrg1, DeclarativeLinkInput{
		Slug:        "test-success",
		Destination: "https://example.com/target",
		Tags:        []string{"promo", "launch"},
	}, nil, nil)
	if errSuccess != nil {
		t.Fatalf("unexpected error on valid creation: %v", errSuccess)
	}
	if outSuccess.ID == 0 || outSuccess.Slug != "test-success" {
		t.Fatalf("unexpected output: %+v", outSuccess)
	}

	// Verify Audit Log
	if len(auditLogs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(auditLogs))
	}
	expectedAudit := fmt.Sprintf("link.create:link:%d:source=declarative", outSuccess.ID)
	if auditLogs[0] != expectedAudit {
		t.Errorf("audit log mismatch: got %q, want %q", auditLogs[0], expectedAudit)
	}

	// Verify Published Event
	if len(eventLogs) != 1 {
		t.Fatalf("expected 1 event log, got %d", len(eventLogs))
	}
	expectedEvent := "1:link.create"
	if eventLogs[0] != expectedEvent {
		t.Errorf("event log mismatch: got %q, want %q", eventLogs[0], expectedEvent)
	}
}

// patchedRenderPasswordGate simulates the proposed remediation in AUDIT_REPORT.md (Finding SEC-03)
func patchedRenderPasswordGate(w http.ResponseWriter, path string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><meta name="referrer" content="no-referrer">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Protected link</title>
<style>body{font-family:system-ui;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:#0b0b0f;color:#fff}
form{background:#16161d;padding:2rem;border-radius:12px;width:300px}
input{width:100%;padding:.6rem;margin:.5rem 0;border-radius:8px;border:1px solid #333;background:#0b0b0f;color:#fff;box-sizing:border-box}
button{width:100%;padding:.6rem;border:0;border-radius:8px;background:#6366f1;color:#fff;font-weight:600;cursor:pointer}</style></head>
<body><form method="post" action="` + html.EscapeString(path) + `">
<h3>🔒 This link is protected</h3>
<input type="password" name="pw" placeholder="Password" autofocus>
<button type="submit">Continue</button></form></body></html>`))
}

// patchedHandle simulates the proposed Engine.Handle remediation in AUDIT_REPORT.md (Finding SEC-03)
func (e *Engine) patchedHandle(w http.ResponseWriter, r *http.Request, link *Link) {
	if expired(link) {
		if link.ExpiredURL != "" {
			w.Header().Set("Referrer-Policy", "no-referrer")
			http.Redirect(w, r, link.ExpiredURL, http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}
	if link.Password != "" {
		provided := r.FormValue("pw")
		if provided == "" {
			provided = r.URL.Query().Get("pw")
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(link.Password)) != 1 {
			patchedRenderPasswordGate(w, r.URL.Path)
			return
		}
	}

	ip := clientIP(r)
	ua := r.UserAgent()
	var country, region, city string
	if e.ctx != nil && e.ctx.GeoLookup != nil {
		country, region, city = e.ctx.GeoLookup(ip)
	}
	var device, browser, osStr string
	if e.ctx != nil && e.ctx.ParseUA != nil {
		device, browser, osStr = e.ctx.ParseUA(ua)
	}
	bot := isBot(ua)

	target := link.Target

	var variant string
	if len(link.RoutingRules) > 0 {
		lang := r.Header.Get("Accept-Language")
		attributeHit := false
		for _, rule := range link.RoutingRules {
			if rule.Type == "split" {
				continue
			}
			if matchRule(rule, country, device, osStr, lang) {
				target = rule.Target
				attributeHit = true
				break
			}
		}
		if !attributeHit {
			fingerprint := deviceFingerprint(anonymizeIP(ip), ua, r.Header.Get("Accept-Language"))
			if splitTarget, splitVariant, ok := splitAssign(link.RoutingRules, fingerprint, link.ID); ok {
				if splitTarget != "" {
					target = splitTarget
				}
				variant = splitVariant
			}
		}
	}

	if e.rateLimiter != nil && !e.rateLimiter.Allow(ip) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	e.record(r, link.OrgID, link.Slug, link.ID, ip, country, region, city, ua, device, browser, osStr, bot, variant)
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, target, http.StatusFound)
}

// TestVerification_SEC03_Remediation tests the refined password gate and Referrer-Policy remediation
func TestVerification_SEC03_Remediation(t *testing.T) {
	t.Parallel()
	s := newTestEngine(t)

	link := &Link{
		OrgID:    1,
		Slug:     "protected-item",
		Target:   "https://target.destination.example.com/secret",
		Enabled:  true,
		Password: "SecretVaultKey_987!",
	}
	s.db.Create(link)

	// 1. Initial GET request without credentials
	recGate := httptest.NewRecorder()
	reqGate := httptest.NewRequest(http.MethodGet, "/protected-item", nil)
	s.patchedHandle(recGate, reqGate, link)

	if recGate.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", recGate.Code)
	}
	if recGate.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("expected Referrer-Policy: no-referrer on 401 gate, got %q", recGate.Header().Get("Referrer-Policy"))
	}
	gateBody := recGate.Body.String()
	if !strings.Contains(gateBody, `<form method="post" action="/protected-item">`) {
		t.Errorf("expected <form method=\"post\" action=\"/protected-item\">, got body:\n%s", gateBody)
	}
	if strings.Contains(gateBody, `method="get"`) {
		t.Errorf("gate HTML still contains method=\"get\"!")
	}
	if !strings.Contains(gateBody, `<meta name="referrer" content="no-referrer">`) {
		t.Errorf("expected <meta name=\"referrer\" content=\"no-referrer\"> in HTML head")
	}

	// 2. Submit password via POST form (application/x-www-form-urlencoded)
	formData := url.Values{}
	formData.Set("pw", "SecretVaultKey_987!")
	reqPost := httptest.NewRequest(http.MethodPost, "/protected-item", strings.NewReader(formData.Encode()))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recPost := httptest.NewRecorder()

	s.patchedHandle(recPost, reqPost, link)

	if recPost.Code != http.StatusFound {
		t.Fatalf("expected 302 Found redirect on valid POST password, got %d", recPost.Code)
	}
	if recPost.Header().Get("Location") != "https://target.destination.example.com/secret" {
		t.Errorf("expected Location https://target.destination.example.com/secret, got %q", recPost.Header().Get("Location"))
	}
	if recPost.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("expected Referrer-Policy: no-referrer on 302 redirect, got %q", recPost.Header().Get("Referrer-Policy"))
	}

	// 3. Submit wrong password via POST form
	formDataWrong := url.Values{}
	formDataWrong.Set("pw", "IncorrectPass!")
	reqPostWrong := httptest.NewRequest(http.MethodPost, "/protected-item", strings.NewReader(formDataWrong.Encode()))
	reqPostWrong.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recPostWrong := httptest.NewRecorder()

	s.patchedHandle(recPostWrong, reqPostWrong, link)
	if recPostWrong.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized on invalid POST password, got %d", recPostWrong.Code)
	}
	if recPostWrong.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("expected Referrer-Policy: no-referrer on 401 wrong pass response")
	}

	// 4. Submit password via GET query string (programmatic link compatibility)
	reqGetPW := httptest.NewRequest(http.MethodGet, "/protected-item?pw=SecretVaultKey_987!", nil)
	recGetPW := httptest.NewRecorder()

	s.patchedHandle(recGetPW, reqGetPW, link)
	if recGetPW.Code != http.StatusFound {
		t.Fatalf("expected 302 Found redirect on GET query pw, got %d", recGetPW.Code)
	}
	if recGetPW.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("expected Referrer-Policy: no-referrer on GET redirect")
	}

	// 5. Expired Link with ExpiredURL redirects with Referrer-Policy
	expiredLink := &Link{
		OrgID:      1,
		Slug:       "expired-item",
		Target:     "https://target.example.com",
		ExpiredURL: "https://fallback.example.com/expired",
		ExpiresAt:  func() *time.Time { t := time.Now().Add(-1 * time.Hour); return &t }(),
		Enabled:    true,
	}
	recExpired := httptest.NewRecorder()
	reqExpired := httptest.NewRequest(http.MethodGet, "/expired-item", nil)
	s.patchedHandle(recExpired, reqExpired, expiredLink)

	if recExpired.Code != http.StatusFound {
		t.Fatalf("expected 302 Found for expired link, got %d", recExpired.Code)
	}
	if recExpired.Header().Get("Location") != "https://fallback.example.com/expired" {
		t.Errorf("expected ExpiredURL redirect location, got %q", recExpired.Header().Get("Location"))
	}
	if recExpired.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("expected Referrer-Policy: no-referrer on expired redirect")
	}
}

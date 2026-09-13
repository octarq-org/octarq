package mail

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

// The generic inbound endpoint authenticates on a token in the URL path, the
// shape n8n and every hosted webhook receiver uses. It has to: this route
// exists for providers whose entire configuration surface is one URL field
// (SendGrid Inbound Parse, Mailgun routes), so requiring a custom header would
// leave them unable to call it at all. The trade is that a URL-borne secret can
// end up in an access log, and the answer to that is rotation — an empty
// inboundToken in Mail settings mints a fresh UUID, and one URL gets repasted.
//
// These tests pin the two properties that make that trade safe: nothing gets in
// without the right token, and a wrong token leaves a trail.

type recordedAudit struct {
	action   string
	targetID uint
	meta     map[string]any
}

// nextInboundAuthOrgID hands out org ids well clear of the low ones other
// tests in this package assign explicitly.
var nextInboundAuthOrgID atomic.Uint32

// setupTestDB hands every test in this package the SAME shared in-memory
// database. Two things follow, and both have already broken a build here:
// a hardcoded slug collides with another test's org, and so does a hardcoded
// id — storage_test.go inserts its org with an explicit ID of 1, which fails
// silently if something else took that id first, leaving its lookup to 404.
// So the slug comes from the test name and the id from a counter starting far
// above anything assigned by hand.
func inboundAuthFixture(t *testing.T) (*Plugin, *[]recordedAudit, string, uint) {
	t.Helper()
	db := setupTestDB(t)
	slug := "inbound-auth-" + strings.ToLower(t.Name())
	org := models.Org{Name: t.Name(), Slug: slug, InboundToken: "right-token"}
	org.ID = 9000 + uint(nextInboundAuthOrgID.Add(1))
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	var seen []recordedAudit
	p := &Plugin{db: db}
	p.audit = func(_ *http.Request, action, _ string, targetID uint, meta map[string]any) {
		seen = append(seen, recordedAudit{action: action, targetID: targetID, meta: meta})
	}
	return p, &seen, slug, org.ID
}

func genericInput(slug, token string) *InboundGenericInput {
	body := []byte("From: a@example.com\r\nTo: bob@example.test\r\nSubject: T\r\n\r\nhi")
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/"+slug+"/email/inbound/raw/"+token, bytes.NewReader(body))
	return &InboundGenericInput{
		Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
		OrgSlug: slug,
		Token:   token,
	}
}

// A header that used to be accepted must not be a way in any more. The token
// travels in the path; anything else is an unauthenticated request.
func TestGenericInboundRejectsHeaderToken(t *testing.T) {
	p, _, slug, _ := inboundAuthFixture(t)

	body := []byte("From: a@example.com\r\nTo: bob@example.test\r\n\r\nhi")
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/"+slug+"/email/inbound/raw/", bytes.NewReader(body))
	req.Header.Set("X-Octarq-Token", "right-token")
	req.Header.Set("Authorization", "Bearer right-token")

	_, err := p.inboundGeneric(context.Background(), &InboundGenericInput{
		Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
		OrgSlug: slug,
		Token:   "", // nothing in the path
	})
	if err == nil {
		t.Fatal("a correct token in a header must not authenticate — the path segment is the credential")
		return
	}
}

func TestGenericInboundRejectsWrongPathToken(t *testing.T) {
	p, _, slug, _ := inboundAuthFixture(t)
	if _, err := p.inboundGeneric(context.Background(), genericInput(slug, "wrong-token")); err == nil {
		t.Fatal("wrong path token must be rejected")
		return
	}
}

// A wrong token has to leave a trail. Guessing the org token buys the ability
// to inject mail into any mailbox in the workspace, so a silent 401 means
// someone can grind the token space indefinitely with nothing to notice.
func TestInboundAuthFailureIsAudited(t *testing.T) {
	p, seen, slug, orgID := inboundAuthFixture(t)

	if _, err := p.inboundGeneric(context.Background(), genericInput(slug, "wrong-token")); err == nil {
		t.Fatal("expected rejection")
		return
	}

	if len(*seen) != 1 {
		t.Fatalf("expected exactly one audit entry for a rejected token, got %d", len(*seen))
	}
	got := (*seen)[0]
	if got.action != "email.inbound.auth_failed" {
		t.Errorf("audit action = %q, want email.inbound.auth_failed", got.action)
	}
	if got.targetID != orgID {
		t.Errorf("audit targetID = %d, want the org id", got.targetID)
	}

	// The attempted token must never be recorded. An operator pasting their
	// real token into the wrong tenant's URL would otherwise write that
	// credential into an audit log they do not control.
	for k, v := range got.meta {
		if s, ok := v.(string); ok && s == "wrong-token" {
			t.Errorf("audit meta[%q] contains the attempted token — it must never be stored", k)
		}
	}
}

// The Cloudflare-worker route carries the same token in its own path segment
// and must audit the same way.
func TestPathTokenRouteAuthFailureIsAudited(t *testing.T) {
	p, seen, slug, _ := inboundAuthFixture(t)

	body := []byte("From: a@example.com\r\nTo: bob@example.test\r\n\r\nhi")
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/"+slug+"/email/inbound/nope", bytes.NewReader(body))
	_, err := p.inbound(context.Background(), &InboundInput{
		Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
		OrgSlug: slug,
		Token:   "nope",
	})
	if err == nil {
		t.Fatal("expected rejection")
		return
	}
	if len(*seen) != 1 || (*seen)[0].action != "email.inbound.auth_failed" {
		t.Fatalf("expected an auth-failure audit entry, got %+v", *seen)
	}
}

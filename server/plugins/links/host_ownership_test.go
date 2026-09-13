package links

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func setupOwnershipTestDB(t *testing.T) (*gorm.DB, *Plugin, *Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&Link{})
	db.Where("1 = 1").Delete(&dns.Domain{})

	p := New()
	p.db = db
	p.auth.OrgID = func(r *http.Request) uint {
		if val := r.Header.Get("X-Org-ID"); val != "" {
			var id uint
			fmt.Sscanf(val, "%d", &id)
			return id
		}
		return 1
	}

	engine := NewEngine(db, mockCtx())
	return db, p, engine
}

func TestHostOwnershipEnforcement(t *testing.T) {
	db, p, engine := setupOwnershipTestDB(t)

	// Org B (orgID = 2) owns victim.com with linkHost "links.victim.com"
	db.Create(&dns.Domain{
		OrgID:   2,
		Name:    "victim.com",
		ForLink: true,
		LinkHosts: models.HostList{
			models.Host{Host: "links.victim.com", Enabled: true},
		},
	})

	// Org A (orgID = 1) owns mybrand.com with linkHost "go.mybrand.com"
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "mybrand.com",
		ForLink: true,
		LinkHosts: models.HostList{
			models.Host{Host: "go.mybrand.com", Enabled: true},
		},
	})

	// Org A (orgID = 1) owns apexonly.com with empty LinkHosts (apex fallback)
	db.Create(&dns.Domain{
		OrgID:     1,
		Name:      "apexonly.com",
		ForLink:   true,
		LinkHosts: nil,
	})

	ctx := context.Background()

	// 1. Org A cannot create host = Org B's linkHost -> 403
	{
		req := httptest.NewRequest(http.MethodPost, "/api/links", nil)
		req.Header.Set("X-Org-ID", "1")
		input := &CreateLinkInput{
			Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: linkDTO{
				Host:   "links.victim.com",
				Slug:   "invoice",
				Target: "https://phish.example",
			},
		}
		_, err := p.createLink(ctx, input)
		if err == nil {
			t.Fatal("expected 403 when Org A creates link on Org B's linkHost, got nil error")
			return
		}
	}

	// 2. Org A cannot update its existing link host to Org B's linkHost -> 403
	var linkA Link
	{
		req := httptest.NewRequest(http.MethodPost, "/api/links", nil)
		req.Header.Set("X-Org-ID", "1")
		input := &CreateLinkInput{
			Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: linkDTO{
				Host:   "go.mybrand.com",
				Slug:   "promo",
				Target: "https://mybrand.example",
			},
		}
		out, err := p.createLink(ctx, input)
		if err != nil {
			t.Fatalf("failed to create valid link for Org A: %v", err)
		}
		linkA = out.Body.Link

		// Now attempt to update linkA's host to links.victim.com
		reqUp := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/links/%d", linkA.ID), nil)
		reqUp.Header.Set("X-Org-ID", "1")
		inputUp := &UpdateLinkInput{
			Ctx: humago.NewContext(nil, reqUp, httptest.NewRecorder()),
			ID:  linkA.ID,
			Body: linkDTO{
				Host:   "links.victim.com",
				Slug:   "promo",
				Target: "https://mybrand.example",
			},
		}
		_, err = p.updateLink(ctx, inputUp)
		if err == nil {
			t.Fatal("expected 403 when Org A updates link host to Org B's linkHost, got nil error")
			return
		}
	}

	// 3. Org A CAN create host = its own linkHost -> 201 (success)
	{
		req := httptest.NewRequest(http.MethodPost, "/api/links", nil)
		req.Header.Set("X-Org-ID", "1")
		input := &CreateLinkInput{
			Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: linkDTO{
				Host:   "go.mybrand.com",
				Slug:   "deal",
				Target: "https://mybrand.example/deal",
			},
		}
		out, err := p.createLink(ctx, input)
		if err != nil {
			t.Fatalf("expected success when Org A creates link on own linkHost, got error: %v", err)
		}
		if out.Body.Host != "go.mybrand.com" {
			t.Errorf("got host %q, want go.mybrand.com", out.Body.Host)
		}
	}

	// 4. Org A CAN create host: "" -> 201 (success)
	{
		req := httptest.NewRequest(http.MethodPost, "/api/links", nil)
		req.Header.Set("X-Org-ID", "1")
		input := &CreateLinkInput{
			Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: linkDTO{
				Host:   "",
				Slug:   "global-link",
				Target: "https://mybrand.example/global",
			},
		}
		out, err := p.createLink(ctx, input)
		if err != nil {
			t.Fatalf("expected success when Org A creates link with empty host, got error: %v", err)
		}
		if out.Body.Host != "" {
			t.Errorf("got host %q, want empty host", out.Body.Host)
		}
	}

	// 5. EffectiveLinkHosts() is empty: Org A uses apex as host -> 201 (fallback semantics success)
	{
		req := httptest.NewRequest(http.MethodPost, "/api/links", nil)
		req.Header.Set("X-Org-ID", "1")
		input := &CreateLinkInput{
			Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: linkDTO{
				Host:   "apexonly.com",
				Slug:   "apex-link",
				Target: "https://apexonly.com/target",
			},
		}
		out, err := p.createLink(ctx, input)
		if err != nil {
			t.Fatalf("expected success when Org A uses apex domain host with empty EffectiveLinkHosts(), got error: %v", err)
		}
		if out.Body.Host != "apexonly.com" {
			t.Errorf("got host %q, want apexonly.com", out.Body.Host)
		}
	}

	// 6. Read side defense-in-depth: Dirty row directly inserted into DB (owner=1, host=links.victim.com)
	{
		dirty := Link{
			OrgID:   1,                  // Org A
			Host:    "links.victim.com", // Org B's host
			Slug:    "dirty-slug",
			Target:  "https://phish.example/dirty",
			Enabled: true,
		}
		if err := db.Create(&dirty).Error; err != nil {
			t.Fatalf("failed to insert dirty link: %v", err)
		}

		// Attempt Lookup on links.victim.com with slug dirty-slug
		_, ok := engine.Lookup("links.victim.com", "dirty-slug")
		if ok {
			t.Fatal("expected Lookup to fail (404) for dirty row on Org B's host owned by Org A, but it succeeded")
		}
	}
}

package links

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func TestPlugin_CreateDeclarativeLink(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)

	// 1. Unauthenticated context
	_, err := p.createDeclarativeLink(context.Background(), DeclarativeLinkInput{
		Destination: "https://example.com/target",
	})
	if err == nil {
		t.Fatalf("expected error for unauthenticated call")
	}
	ae, ok := plugin.AsAgentError(err)
	if !ok || ae.Code != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED AgentError, got %v", err)
	}

	// 2. Authenticated context with empty destination
	ctxOrg := plugin.WithOrgID(context.Background(), 1)
	_, err = p.createDeclarativeLink(ctxOrg, DeclarativeLinkInput{
		Destination: "   ",
	})
	if err == nil {
		t.Fatalf("expected error for empty destination")
	}
	ae, ok = plugin.AsAgentError(err)
	if !ok || ae.Code != "MISSING_DESTINATION" {
		t.Errorf("expected MISSING_DESTINATION AgentError, got %v", err)
	}

	// 3. Authenticated context with invalid destination URL
	_, err = p.createDeclarativeLink(ctxOrg, DeclarativeLinkInput{
		Destination: "ftp://invalid-scheme.com",
	})
	if err == nil {
		t.Fatalf("expected error for invalid destination scheme")
	}
	ae, ok = plugin.AsAgentError(err)
	if !ok || ae.Code != "INVALID_DESTINATION" {
		t.Errorf("expected INVALID_DESTINATION AgentError, got %v", err)
	}

	// 4. Authenticated context with reserved slug
	_, err = p.createDeclarativeLink(ctxOrg, DeclarativeLinkInput{
		Destination: "https://example.com",
		Slug:        "admin",
	})
	if err == nil {
		t.Fatalf("expected error for reserved slug")
	}
	ae, ok = plugin.AsAgentError(err)
	if !ok || ae.Code != "SLUG_RESERVED" {
		t.Errorf("expected SLUG_RESERVED AgentError, got %v", err)
	}

	// 5. Successful link creation
	out, err := p.createDeclarativeLink(ctxOrg, DeclarativeLinkInput{
		Destination: "https://example.com/landing",
		Slug:        "my-promo",
		Tags:        []string{"promo", "summer"},
	})
	if err != nil {
		t.Fatalf("unexpected error creating declarative link: %v", err)
	}
	if out.ID == 0 || out.Slug != "my-promo" || out.Destination != "https://example.com/landing" {
		t.Errorf("unexpected output: %+v", out)
	}

	// 6. Duplicate slug creation attempt
	_, err = p.createDeclarativeLink(ctxOrg, DeclarativeLinkInput{
		Destination: "https://example.com/other",
		Slug:        "my-promo",
	})
	if err == nil {
		t.Fatalf("expected error for duplicate slug")
	}
	ae, ok = plugin.AsAgentError(err)
	if !ok || ae.Code != "SLUG_ALREADY_EXISTS" {
		t.Errorf("expected SLUG_ALREADY_EXISTS AgentError, got %v", err)
	}
}

func TestPlugin_MountRegistersDeclarativeEndpoint(t *testing.T) {
	p, _ := setupFullLinksTestDB(t)

	var registeredSpec any
	pctx := &plugin.Context{
		RegisterEndpoint: func(spec any) error {
			registeredSpec = spec
			return nil
		},
	}

	p.Mount(nil, pctx)

	if registeredSpec == nil {
		t.Fatalf("expected RegisterEndpoint to be called during Mount")
	}

	ep, ok := registeredSpec.(plugin.Endpoint)
	if !ok {
		t.Fatalf("expected registered spec to implement plugin.Endpoint")
	}
	if ep.EndpointName() != "create_shortlink" {
		t.Errorf("expected endpoint name 'create_shortlink', got %q", ep.EndpointName())
	}
	if ep.EndpointMethod() != "POST" {
		t.Errorf("expected endpoint method 'POST', got %q", ep.EndpointMethod())
	}
	if ep.EndpointPath() != "/api/links/declarative" {
		t.Errorf("expected endpoint path '/api/links/declarative', got %q", ep.EndpointPath())
	}
	if !ep.EndpointExposeMCP() {
		t.Errorf("expected ExposeMCP to be true")
	}
}

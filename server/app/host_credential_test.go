package app

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// stubResolver records what the host asked it for and answers with a fixed
// result.
type stubResolver struct {
	cred     *plugin.Credential
	err      error
	gotOrg   uint
	gotQuery plugin.CredentialQuery
	called   bool
}

func (s *stubResolver) LookupCredential(_ context.Context, orgID uint, query plugin.CredentialQuery) (*plugin.Credential, error) {
	s.called = true
	s.gotOrg = orgID
	s.gotQuery = query
	return s.cred, s.err
}

// The two sentinels have different meanings, and callers act on the
// difference: only "unavailable" lets a plugin read its own local registry.
// "Not found" from a mounted resolver is a final answer.

func TestHostRuntime_LookupCredentialReportsUnavailableWithoutAResolver(t *testing.T) {
	var nilRuntime *hostRuntime
	if _, err := nilRuntime.LookupCredential(context.Background(), 1, plugin.CredentialQuery{}); !errors.Is(err, plugin.ErrCredentialResolverUnavailable) {
		t.Fatalf("nil runtime: expected ErrCredentialResolverUnavailable, got %v", err)
	}

	empty := &hostRuntime{}
	if _, err := empty.LookupCredential(context.Background(), 1, plugin.CredentialQuery{}); !errors.Is(err, plugin.ErrCredentialResolverUnavailable) {
		t.Fatalf("no services: expected ErrCredentialResolverUnavailable, got %v", err)
	}

	// A registry with no resolver is the standalone Core case, not a miss.
	withoutResolver := &hostRuntime{services: plugin.NewRegistry()}
	if _, err := withoutResolver.LookupCredential(context.Background(), 1, plugin.CredentialQuery{}); !errors.Is(err, plugin.ErrCredentialResolverUnavailable) {
		t.Fatalf("empty registry: expected ErrCredentialResolverUnavailable, got %v", err)
	}
}

func TestHostRuntime_LookupCredentialDelegatesToTheResolver(t *testing.T) {
	reg := plugin.NewRegistry()
	want := &plugin.Credential{ID: 3, OrgID: 7, ProviderType: "dns", Provider: "cloudflare"}
	resolver := &stubResolver{cred: want}
	reg.Provide(plugin.ServiceCredentialResolver, resolver)

	h := &hostRuntime{services: reg}
	query := plugin.CredentialQuery{ProviderType: "dns", Provider: "cloudflare"}

	got, err := h.LookupCredential(context.Background(), 7, query)
	if err != nil {
		t.Fatalf("LookupCredential: %v", err)
	}
	if got != want {
		t.Errorf("expected the resolver's credential, got %+v", got)
	}
	if !resolver.called || resolver.gotOrg != 7 || resolver.gotQuery != query {
		t.Errorf("expected the lookup to reach the resolver unchanged, got %+v", resolver)
	}

	// A resolver miss is reported as not found, never as unavailable.
	resolver.err = plugin.ErrCredentialNotFound
	if _, err := h.LookupCredential(context.Background(), 7, query); !errors.Is(err, plugin.ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound to pass through, got %v", err)
	}

	// Any other failure also passes through: the caller must not mistake a
	// broken resolver for a workspace with no integrations.
	resolver.err = errors.New("database is locked")
	if _, err := h.LookupCredential(context.Background(), 7, query); err == nil || errors.Is(err, plugin.ErrCredentialResolverUnavailable) || errors.Is(err, plugin.ErrCredentialNotFound) {
		t.Errorf("expected the resolver error to pass through unchanged, got %v", err)
	}
}

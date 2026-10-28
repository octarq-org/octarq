package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// TestSyncDomains_ResolvesThroughTheHubOnly pins the reason sync reads the
// shared resolver: a hub-backed account must sync, and a same-numbered local
// row must not answer for it. Before this, every hub account returned 404.
func TestSyncDomains_ResolvesThroughTheHubOnly(t *testing.T) {
	p, mkCtx := setupFreshTestDB(t)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := mkCtx(req)

	// A local row with a usable config exists under the same id the caller sends.
	if err := p.db.Create(&ProviderAccount{
		ID: 50, OrgID: 1, Name: "Local account", Type: "cloudflare",
		Config: `{"apiToken":"local"}`,
	}).Error; err != nil {
		t.Fatalf("seed local account: %v", err)
	}

	input := &SyncDomainsInput{Ctx: ctx}
	input.Body.ProviderAccountID = 50

	// The hub is mounted and owns DNS credentials, but has no such account.
	p.host = &plugin.TestHost{
		LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
			return nil, plugin.ErrCredentialNotFound
		},
	}
	if _, err := p.syncDomains(context.Background(), input); err == nil {
		t.Fatal("expected the hub's answer to stand: no local fallback during sync")
	} else if status := statusOf(t, err); status != http.StatusNotFound {
		t.Fatalf("expected 404 for an account the hub does not know, got %d (%v)", status, err)
	}

	// The hub knows the account but hands over a provider that cannot be built.
	p.host = &plugin.TestHost{
		LookupCredentialMock: func(_ context.Context, orgID uint, _ plugin.CredentialQuery) (*plugin.Credential, error) {
			return &plugin.Credential{
				ID: 50, OrgID: orgID, Provider: "not-a-provider",
				ConfigJSON: []byte(`{}`),
			}, nil
		},
	}
	if _, err := p.syncDomains(context.Background(), input); err == nil {
		t.Fatal("expected an unusable hub credential to fail the sync")
	}
}

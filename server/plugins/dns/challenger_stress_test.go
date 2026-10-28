package dns

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// TestChallenger_DNSCredentialBridgeStress stress tests DNS credential resolution,
// cross-tenant isolation, fallback hierarchy, and invalid configurations.
func TestChallenger_DNSCredentialBridgeStress(t *testing.T) {
	p, _ := setupFreshTestDB(t)

	// Mock Host with Org 10 having ID 88 (Cloudflare valid),
	// Org 10 having ID 89 (Cloudflare missing apiToken),
	// Org 10 having ID 90 (Cloudflare malformed JSON),
	// Org 20 having ID 77 (Cloudflare valid).
	th := &plugin.TestHost{
		LookupCredentialMock: func(ctx context.Context, orgID uint, query plugin.CredentialQuery) (*plugin.Credential, error) {
			if query.ProviderType != "dns" && query.ProviderType != "" {
				return nil, plugin.ErrCredentialNotFound
			}
			switch orgID {
			case 10:
				switch query.ID {
				case 88:
					return &plugin.Credential{
						ID:           88,
						OrgID:        10,
						ProviderType: "dns",
						Provider:     "cloudflare",
						Name:         "Org 10 Valid CF",
						ConfigJSON:   json.RawMessage(`{"apiToken":"org10-cf-token"}`),
					}, nil
				case 89:
					// Missing apiToken
					return &plugin.Credential{
						ID:           89,
						OrgID:        10,
						ProviderType: "dns",
						Provider:     "cloudflare",
						Name:         "Org 10 Missing Token",
						ConfigJSON:   json.RawMessage(`{"apiToken":""}`),
					}, nil
				case 90:
					// Malformed JSON
					return &plugin.Credential{
						ID:           90,
						OrgID:        10,
						ProviderType: "dns",
						Provider:     "cloudflare",
						Name:         "Org 10 Corrupt JSON",
						ConfigJSON:   json.RawMessage(`{"apiToken": `),
					}, nil
				case 91:
					// Empty ConfigJSON
					return &plugin.Credential{
						ID:           91,
						OrgID:        10,
						ProviderType: "dns",
						Provider:     "cloudflare",
						Name:         "Org 10 Empty Config",
						ConfigJSON:   nil,
					}, nil
				}
			case 20:
				if query.ID == 77 {
					return &plugin.Credential{
						ID:           77,
						OrgID:        20,
						ProviderType: "dns",
						Provider:     "cloudflare",
						Name:         "Org 20 Valid CF",
						ConfigJSON:   json.RawMessage(`{"apiToken":"org20-cf-token"}`),
					}, nil
				}
			}
			return nil, plugin.ErrCredentialNotFound
		},
	}
	p.host = th

	// Seed local OSS table for fallback testing
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
	p.db.Create(&ProviderAccount{
		ID:     50,
		OrgID:  10,
		Name:   "Local OSS Org 10 Account",
		Type:   "cloudflare",
		Config: `{"apiToken":"oss-token-org10"}`,
	})
	p.db.Create(&ProviderAccount{
		ID:     60,
		OrgID:  20,
		Name:   "Local OSS Org 20 Account",
		Type:   "cloudflare",
		Config: `{"apiToken":"oss-token-org20"}`,
	})

	// -------------------------------------------------------------
	// 1. Cross-Tenant Isolation Tests
	// -------------------------------------------------------------
	t.Run("CrossTenant_ownsProviderAccount", func(t *testing.T) {
		p.orgID = func(r *http.Request) uint { return 20 }
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		// Org 20 attempts to own Org 10's SPI account (ID 88)
		if p.ownsProviderAccount(req, 88) {
			t.Errorf("CRITICAL SECURITY FAILURE: ownsProviderAccount(88) returned true for Org 20")
		}

		// Org 20 attempts to own Org 10's local account (ID 50)
		if p.ownsProviderAccount(req, 50) {
			t.Errorf("CRITICAL SECURITY FAILURE: ownsProviderAccount(50) returned true for Org 20")
		}

		// Org 20 owns its own SPI account (ID 77)
		if !p.ownsProviderAccount(req, 77) {
			t.Errorf("ownsProviderAccount(77) failed for Org 20")
		}

		// Org 20 owns its own local account (ID 60)
		if !p.ownsProviderAccount(req, 60) {
			t.Errorf("ownsProviderAccount(60) failed for Org 20")
		}
	})

	t.Run("CrossTenant_providerFor", func(t *testing.T) {
		// Domain owned by Org 20, but referencing Org 10's SPI account (ID 88)
		dom := Domain{
			OrgID:             20,
			Name:              "cross-test.com",
			ProviderAccountID: 88,
		}
		prov, err := p.providerFor(dom)
		if err == nil {
			t.Errorf("CRITICAL SECURITY FAILURE: providerFor resolved Org 10's credential for Org 20! prov=%+v", prov)
		}
		if !strings.Contains(err.Error(), "provider account not found") {
			t.Errorf("expected 'provider account not found', got: %v", err)
		}
	})

	// -------------------------------------------------------------
	// 2. Fallback Hierarchy Tests
	// -------------------------------------------------------------
	t.Run("FallbackHierarchy", func(t *testing.T) {
		// Scenario A: Host SPI present -> resolved from SPI
		provSPI, err := p.providerFor(Domain{OrgID: 10, ProviderAccountID: 88})
		if err != nil || provSPI == nil {
			t.Fatalf("expected SPI resolution to succeed, err=%v", err)
		}

		// Scenario B: Host SPI returns ErrCredentialNotFound -> resolved from local table
		provLocal, err := p.providerFor(Domain{OrgID: 10, ProviderAccountID: 50})
		if err != nil || provLocal == nil {
			t.Fatalf("expected fallback to local table to succeed, err=%v", err)
		}

		// Scenario C: Absent everywhere -> fails closed
		_, err = p.providerFor(Domain{OrgID: 10, ProviderAccountID: 9999})
		if err == nil {
			t.Fatalf("expected error for non-existent account, got nil")
		}
		if !strings.Contains(err.Error(), "provider account not found") {
			t.Errorf("expected 'provider account not found', got: %v", err)
		}

		// Scenario D: ProviderAccountID == 0 -> fails closed
		_, err = p.providerFor(Domain{OrgID: 10, ProviderAccountID: 0})
		if err == nil || !strings.Contains(err.Error(), "has no provider account configured") {
			t.Errorf("expected 'has no provider account configured', got: %v", err)
		}

		// Scenario E: Standalone Core OSS (p.host == nil) -> resolves from local table
		pStandalone, _ := setupFreshTestDB(t)
		pStandalone.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
		pStandalone.db.Create(&ProviderAccount{
			ID:     12,
			OrgID:  10,
			Name:   "Standalone Account",
			Type:   "cloudflare",
			Config: `{"apiToken":"oss-token"}`,
		})
		provOSS, err := pStandalone.providerFor(Domain{OrgID: 10, ProviderAccountID: 12})
		if err != nil || provOSS == nil {
			t.Fatalf("standalone providerFor failed: %v", err)
		}
	})

	// -------------------------------------------------------------
	// 3. Invalid Configurations Tests
	// -------------------------------------------------------------
	t.Run("InvalidConfigurations_MissingAPIToken", func(t *testing.T) {
		// ID 89 has empty apiToken in Host SPI
		_, err := p.providerFor(Domain{OrgID: 10, ProviderAccountID: 89})
		if err == nil {
			t.Fatalf("expected error for empty apiToken in providerFor, got nil")
		}
		if !strings.Contains(err.Error(), "apiToken required") {
			t.Errorf("expected 'apiToken required' error, got: %v", err)
		}
	})

	t.Run("InvalidConfigurations_MalformedJSON", func(t *testing.T) {
		// ID 90 has malformed JSON in Host SPI
		_, err := p.providerFor(Domain{OrgID: 10, ProviderAccountID: 90})
		if err == nil {
			t.Fatalf("expected error for malformed JSON in providerFor, got nil")
		}
		if !strings.Contains(err.Error(), "parse cloudflare creds") {
			t.Errorf("expected 'parse cloudflare creds' error, got: %v", err)
		}
	})

	t.Run("InvalidConfigurations_EmptyConfigJSON", func(t *testing.T) {
		// ID 91 has empty ConfigJSON (len == 0).
		// In providerFor:
		// if err == nil && cred != nil && len(cred.ConfigJSON) > 0 { return ... }
		// Because len(cred.ConfigJSON) == 0, it falls through to local table!
		// Since ID 91 does not exist in local table, it returns "provider account not found".
		_, err := p.providerFor(Domain{OrgID: 10, ProviderAccountID: 91})
		if err == nil {
			t.Fatalf("expected error for empty ConfigJSON in providerFor, got nil")
		}
		t.Logf("Empty ConfigJSON behavior: err=%v", err)
	})
}

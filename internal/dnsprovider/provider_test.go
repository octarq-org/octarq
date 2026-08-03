package dnsprovider

import (
	"testing"
)

func TestProviderRegistryAndConstructors(t *testing.T) {
	t.Parallel()

	// 1. Unknown provider
	if _, err := New("unknown_prov", nil); err == nil {
		t.Error("expected error for unknown provider")
	}

	// 2. Cloudflare invalid JSON
	if _, err := New("cloudflare", []byte("bad json")); err == nil {
		t.Error("expected error for bad cloudflare json")
	}

	// 3. Cloudflare missing token
	if _, err := New("cloudflare", []byte("{}")); err == nil {
		t.Error("expected error for missing apiToken")
	}

	// 4. Cloudflare valid token
	p, err := New("cloudflare", []byte(`{"apiToken":"test-token"}`))
	if err != nil || p == nil {
		t.Errorf("cloudflare constructor failed: %v", err)
	}

	// 5. DNSPod missing token/secrets
	if _, err := New("dnspod", []byte("{}")); err == nil {
		t.Error("expected error for missing dnspod secrets")
	}

	// 6. DNSPod token split form
	p2, err := New("dnspod", []byte(`{"token":"id123,key456"}`))
	if err != nil || p2 == nil {
		t.Errorf("dnspod token constructor failed: %v", err)
	}

	// 7. MarshalCreds & Names
	names := Names()
	if len(names) < 2 {
		t.Errorf("expected at least 2 registered providers, got %v", names)
	}

	b, err := MarshalCreds(map[string]string{"apiToken": "xyz"})
	if err != nil || len(b) == 0 {
		t.Fatalf("MarshalCreds error: %v", err)
	}
}

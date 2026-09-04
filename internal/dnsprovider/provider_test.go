package dnsprovider

import (
	"errors"
	"sort"
	"testing"
)

type dummyProvider struct{ Provider }

func TestRegisterAndNew(t *testing.T) {
	// Not using t.Parallel() because Register modifies global state (registry)

	dummyName := "test_dummy_provider"

	Register(dummyName, func(credsJSON []byte) (Provider, error) {
		if string(credsJSON) == "error" {
			return nil, errors.New("simulated error")
		}
		return &dummyProvider{}, nil
	})

	// Test successful creation
	p, err := New(dummyName, []byte("valid"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*dummyProvider); !ok {
		t.Fatalf("expected provider to be of type *dummyProvider, got %T", p)
	}

	// Test factory returning error
	_, err = New(dummyName, []byte("error"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "simulated error" {
		t.Fatalf("expected simulated error, got: %v", err)
	}

	// Test unknown provider error
	p2, err := New("non_existent_provider_xyz123", nil)
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if p2 != nil {
		t.Fatalf("expected nil provider, got %T", p2)
	}
}

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

	// 6. DNSPod secretId/secretKey form
	p2, err := New("dnspod", []byte(`{"secretId":"id123","secretKey":"key456"}`))
	if err != nil || p2 == nil {
		t.Errorf("dnspod secretId/secretKey constructor failed: %v", err)
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

func TestNames(t *testing.T) {
	// Not using t.Parallel() because Register modifies global state (registry)

	// Register some mock providers out of order
	Register("mock_c", func([]byte) (Provider, error) { return nil, nil })
	Register("mock_a", func([]byte) (Provider, error) { return nil, nil })
	Register("mock_b", func([]byte) (Provider, error) { return nil, nil })

	names := Names()

	// Ensure the returned names are sorted
	if !sort.StringsAreSorted(names) {
		t.Errorf("expected Names() to return a sorted slice, got %v", names)
	}

	// Check that our registered mock providers exist in the list
	expectedMocks := []string{"mock_a", "mock_b", "mock_c"}
	for _, mock := range expectedMocks {
		found := false
		for _, name := range names {
			if name == mock {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected Names() to contain %q, but it was missing", mock)
		}
	}
}

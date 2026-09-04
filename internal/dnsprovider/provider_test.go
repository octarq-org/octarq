package dnsprovider

import (
	"errors"
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

	// 7. Names
	names := Names()
	if len(names) < 2 {
		t.Errorf("expected at least 2 registered providers, got %v", names)
	}
}

func TestMarshalCreds(t *testing.T) {
	t.Parallel()

	// 1. Valid map
	b, err := MarshalCreds(map[string]string{"apiToken": "xyz"})
	if err != nil || len(b) == 0 {
		t.Fatalf("MarshalCreds error: %v", err)
	}
	if string(b) != `{"apiToken":"xyz"}` {
		t.Errorf("unexpected marshaled result: %s", string(b))
	}

	// 2. Valid struct
	type creds struct {
		Secret string `json:"secret"`
	}
	b2, err := MarshalCreds(creds{Secret: "foo"})
	if err != nil || len(b2) == 0 {
		t.Fatalf("MarshalCreds struct error: %v", err)
	}
	if string(b2) != `{"secret":"foo"}` {
		t.Errorf("unexpected marshaled result: %s", string(b2))
	}

	// 3. Error path (e.g. unsupported type like channel)
	_, err = MarshalCreds(make(chan int))
	if err == nil {
		t.Error("expected error marshaling channel")
	}
}

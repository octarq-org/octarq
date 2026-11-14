package api

import (
	"errors"
	"sync"
	"testing"

	"github.com/octarq-org/octarq/server/llmprovider"
)

func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OCTARQ_LLM_PROVIDER",
		"OCTARQ_LLM_API_KEY",
		"ANTHROPIC_API_KEY",
		"OCTARQ_LLM_BASE_URL",
	} {
		t.Setenv(key, "")
	}
}

func TestLLMProviderForOrgFailsClosedWithoutConfiguration(t *testing.T) {
	clearLLMEnv(t)

	cases := map[string]*Handler{
		"nil resolver": {},
		// An injected resolver can break its contract and answer (nil, nil).
		// The getter promises an error whenever no backend is usable.
		"resolver returns no provider": {
			llmResolver: func(uint) (llmprovider.Provider, error) { return nil, nil },
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := h.LLMProviderForOrg(1)
			if err == nil {
				t.Fatal("LLMProviderForOrg returned nil error, want errLLMNotConfigured")
			}
			if !errors.Is(err, errLLMNotConfigured) {
				t.Fatalf("LLMProviderForOrg error %v does not wrap errLLMNotConfigured", err)
			}
			if p != nil {
				t.Fatalf("LLMProviderForOrg returned provider %v with an error, want nil", p)
			}
		})
	}
}

func TestLLMProviderForOrgDelegatesPerOrg(t *testing.T) {
	want7 := namedLLM{name: "provider-org-7"}
	want42 := namedLLM{name: "provider-org-42"}

	var mu sync.Mutex
	seen := map[uint]bool{}
	h := &Handler{}
	h.SetLLMResolverForOrg(func(orgID uint) (llmprovider.Provider, error) {
		mu.Lock()
		seen[orgID] = true
		mu.Unlock()
		switch orgID {
		case 7:
			return want7, nil
		case 42:
			return want42, nil
		default:
			return nil, errLLMNotConfigured
		}
	})

	got7, err7 := h.LLMProviderForOrg(7)
	if err7 != nil {
		t.Fatalf("LLMProviderForOrg(7) error: %v", err7)
	}
	if got7 != want7 {
		t.Fatalf("LLMProviderForOrg(7) = %v, want exactly provider-org-7", got7)
	}

	got42, err42 := h.LLMProviderForOrg(42)
	if err42 != nil {
		t.Fatalf("LLMProviderForOrg(42) error: %v", err42)
	}
	if got42 != want42 {
		t.Fatalf("LLMProviderForOrg(42) = %v, want exactly provider-org-42", got42)
	}

	if got7 == got42 {
		t.Fatal("LLMProviderForOrg returned the same backend for org 7 and org 42")
	}

	mu.Lock()
	gotSeen7, gotSeen42 := seen[7], seen[42]
	mu.Unlock()
	if !gotSeen7 || !gotSeen42 {
		t.Fatalf("resolver saw orgs %v, want 7 and 42", seen)
	}

	p, err := h.LLMProviderForOrg(99)
	if err == nil || !errors.Is(err, errLLMNotConfigured) || p != nil {
		t.Fatalf("LLMProviderForOrg(99) = (%v, %v), want (nil, errLLMNotConfigured)", p, err)
	}
}

func TestLLMProviderForOrgConcurrentWithSetter(t *testing.T) {
	h := &Handler{llmResolver: func(uint) (llmprovider.Provider, error) {
		return nil, errLLMNotConfigured
	}}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = h.LLMProviderForOrg(uint(j))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 200; j++ {
			h.SetLLMResolverForOrg(func(uint) (llmprovider.Provider, error) {
				return namedLLM{name: "swapped"}, nil
			})
		}
	}()
	wg.Wait()
}

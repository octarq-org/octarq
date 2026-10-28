package plugin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// TestEnsureHostCredentialLookupFailsClosed pins the sentinel a plugin sees when
// no resolver is mounted. "Unavailable" is the only answer that lets a plugin
// read its own local registry; a bare error would make it fail instead.
func TestEnsureHostCredentialLookupFailsClosed(t *testing.T) {
	h := plugin.EnsureHost(nil)

	if _, err := h.LookupCredential(context.Background(), 1, plugin.CredentialQuery{ProviderType: "mail"}); !errors.Is(err, plugin.ErrCredentialResolverUnavailable) {
		t.Fatalf("expected ErrCredentialResolverUnavailable, got %v", err)
	}

	// A host with no workspace behind it publishes no harness, and says so.
	raw, err := h.HarnessSpec(context.Background(), 1)
	if err != nil || raw != nil {
		t.Fatalf("expected no published harness, got %q / %v", raw, err)
	}

	// The no-op event spine accepts calls without panicking.
	h.Events().PublishEvent(1, "test.event", nil)
	h.Events().RegisterWebhookEvent(plugin.WebhookEventDef{})
	h.Events().OnEmail(func(plugin.EmailEvent) {})
}

// TestTestHostDefaultsAreFailClosed keeps the test host honest: tests that do
// not mount a resolver must see the same "unavailable" answer production does,
// not a "not found" that hides a missing mock.
func TestTestHostDefaultsAreFailClosed(t *testing.T) {
	th := &plugin.TestHost{}

	if _, err := th.LookupCredential(context.Background(), 1, plugin.CredentialQuery{}); !errors.Is(err, plugin.ErrCredentialResolverUnavailable) {
		t.Fatalf("expected ErrCredentialResolverUnavailable, got %v", err)
	}

	raw, err := th.HarnessSpec(context.Background(), 1)
	if err != nil || raw != nil {
		t.Fatalf("expected no published harness, got %q / %v", raw, err)
	}
}

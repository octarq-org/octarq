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

// stubWorkforce is a ledger a test can hand to TestHost.
type stubWorkforce struct{ calls int }

func (s *stubWorkforce) AuthorizeTurn(context.Context, uint, string, string) error {
	s.calls++
	return nil
}
func (s *stubWorkforce) TurnCost(context.Context, uint, string) (func(input, output, reasoning int) float64, error) {
	return func(int, int, int) float64 { return 0 }, nil
}
func (s *stubWorkforce) RecordSpend(context.Context, uint, plugin.TurnSpend) (*plugin.SpendRecord, error) {
	return &plugin.SpendRecord{}, nil
}
func (s *stubWorkforce) DecideApproval(context.Context, uint, string, bool) error { return nil }

// TestTestHostWorkforceUsesTheMock pins that a test which supplies a ledger
// gets that ledger, and that a host variable which is nil still answers with
// the unavailable sentinel instead of panicking.
func TestTestHostWorkforceUsesTheMock(t *testing.T) {
	mock := &stubWorkforce{}
	th := &plugin.TestHost{WorkforceMock: mock}
	if got := th.Workforce(); got != plugin.Workforce(mock) {
		t.Fatalf("Workforce = %T, want the mock", got)
	}
	if err := th.Workforce().AuthorizeTurn(context.Background(), 1, "a", "m"); err != nil {
		t.Fatalf("AuthorizeTurn = %v, want nil", err)
	}
	if mock.calls != 1 {
		t.Fatalf("mock saw %d calls, want 1", mock.calls)
	}

	var nilHost *plugin.TestHost
	if err := nilHost.Workforce().AuthorizeTurn(context.Background(), 1, "a", "m"); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
		t.Fatalf("nil host: AuthorizeTurn = %v, want ErrWorkforceUnavailable", err)
	}
}

// TestWorkforceWithoutALedgerRefusesEveryCall pins the host that has no agent
// ledger: every method refuses with ErrWorkforceUnavailable. A zero-cost
// answer would let a turn run that nobody can bill.
func TestWorkforceWithoutALedgerRefusesEveryCall(t *testing.T) {
	ctx := context.Background()
	for name, w := range map[string]plugin.Workforce{
		"noop host": plugin.EnsureHost(nil).Workforce(),
		"test host": (&plugin.TestHost{}).Workforce(),
	} {
		if err := w.AuthorizeTurn(ctx, 1, "agent", "model"); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
			t.Fatalf("%s: AuthorizeTurn = %v, want ErrWorkforceUnavailable", name, err)
		}
		if _, err := w.TurnCost(ctx, 1, "model"); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
			t.Fatalf("%s: TurnCost = %v, want ErrWorkforceUnavailable", name, err)
		}
		if _, err := w.RecordSpend(ctx, 1, plugin.TurnSpend{AgentID: "agent"}); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
			t.Fatalf("%s: RecordSpend = %v, want ErrWorkforceUnavailable", name, err)
		}
		if err := w.DecideApproval(ctx, 1, "task", true); !errors.Is(err, plugin.ErrWorkforceUnavailable) {
			t.Fatalf("%s: DecideApproval = %v, want ErrWorkforceUnavailable", name, err)
		}
	}
}

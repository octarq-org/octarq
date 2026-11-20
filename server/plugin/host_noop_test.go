package plugin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestEnsureHostCredentialLookupFailsClosed(t *testing.T) {
	h := plugin.EnsureHost(nil)

	if _, err := h.LookupCredential(context.Background(), 1, plugin.CredentialQuery{ProviderType: "mail"}); !errors.Is(err, plugin.ErrCredentialResolverUnavailable) {
		t.Fatalf("expected ErrCredentialResolverUnavailable, got %v", err)
	}

	raw, err := h.HarnessSpec(context.Background(), 1)
	if err != nil || raw != nil {
		t.Fatalf("expected no published harness, got %q / %v", raw, err)
	}

	h.Events().PublishEvent(1, "test.event", nil)
	h.Events().RegisterWebhookEvent(plugin.WebhookEventDef{})
	h.Events().OnEmail(func(plugin.EmailEvent) {})
}

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

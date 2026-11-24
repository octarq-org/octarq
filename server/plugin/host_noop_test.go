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

type stubSpendLedger struct{ calls int }

func (s *stubSpendLedger) AuthorizeTurn(context.Context, uint, string, string) error {
	s.calls++
	return nil
}
func (s *stubSpendLedger) TurnCost(context.Context, uint, string) (func(input, output, cacheRead, cacheWrite int) float64, error) {
	return func(int, int, int, int) float64 { return 0 }, nil
}
func (s *stubSpendLedger) RecordSpend(context.Context, uint, plugin.TurnSpend) (*plugin.SpendRecord, error) {
	return &plugin.SpendRecord{}, nil
}
func (*stubSpendLedger) IsVirtualKey(string) bool { return true }

func (s *stubSpendLedger) ResolveVirtualKey(context.Context, string) (*plugin.SpendAccountAuth, error) {
	return &plugin.SpendAccountAuth{OrgID: 1, AccountID: "acc", Status: "active"}, nil
}

func TestTestHostSpendLedgerUsesTheMock(t *testing.T) {
	mock := &stubSpendLedger{}
	th := &plugin.TestHost{SpendLedgerMock: mock}
	if got := th.SpendLedger(); got != plugin.SpendLedger(mock) {
		t.Fatalf("SpendLedger = %T, want the mock", got)
	}
	if err := th.SpendLedger().AuthorizeTurn(context.Background(), 1, "a", "m"); err != nil {
		t.Fatalf("AuthorizeTurn = %v, want nil", err)
	}
	if mock.calls != 1 {
		t.Fatalf("mock saw %d calls, want 1", mock.calls)
	}

	var nilHost *plugin.TestHost
	if err := nilHost.SpendLedger().AuthorizeTurn(context.Background(), 1, "a", "m"); !errors.Is(err, plugin.ErrSpendUnavailable) {
		t.Fatalf("nil host: AuthorizeTurn = %v, want ErrSpendUnavailable", err)
	}
}

func TestSpendLedgerWithoutALedgerRefusesEveryCall(t *testing.T) {
	ctx := context.Background()
	for name, l := range map[string]plugin.SpendLedger{
		"noop host": plugin.EnsureHost(nil).SpendLedger(),
		"test host": (&plugin.TestHost{}).SpendLedger(),
	} {
		if err := l.AuthorizeTurn(ctx, 1, "account", "model"); !errors.Is(err, plugin.ErrSpendUnavailable) {
			t.Fatalf("%s: AuthorizeTurn = %v, want ErrSpendUnavailable", name, err)
		}
		if _, err := l.TurnCost(ctx, 1, "model"); !errors.Is(err, plugin.ErrSpendUnavailable) {
			t.Fatalf("%s: TurnCost = %v, want ErrSpendUnavailable", name, err)
		}
		if _, err := l.RecordSpend(ctx, 1, plugin.TurnSpend{AccountID: "account"}); !errors.Is(err, plugin.ErrSpendUnavailable) {
			t.Fatalf("%s: RecordSpend = %v, want ErrSpendUnavailable", name, err)
		}
		if _, err := l.ResolveVirtualKey(ctx, "sk-octarq-test"); !errors.Is(err, plugin.ErrSpendUnavailable) {
			t.Fatalf("%s: ResolveVirtualKey = %v, want ErrSpendUnavailable", name, err)
		}
	}
}

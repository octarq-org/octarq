package harness

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/llmprovider"
)

// tokenCompleter reports token usage and asks for a tool every round, so a
// budget test can drive the loop without a real provider.
type tokenCompleter struct {
	input  int
	output int
	calls  int
}

func (c *tokenCompleter) CompleteResult(context.Context, string, string, []Message) (CompletionResult, error) {
	c.calls++
	return CompletionResult{
		Text:         toolCallBlock("noop", "{}"),
		InputTokens:  c.input,
		OutputTokens: c.output,
	}, nil
}

func (c *tokenCompleter) Complete(ctx context.Context, model, system string, msgs []Message) (string, error) {
	res, err := c.CompleteResult(ctx, model, system, msgs)
	return res.Text, err
}

func newBudgetTurn() (*Session, *Turn) {
	return &Session{ID: "s", OrgID: 1}, &Turn{Input: "go"}
}

// TestBudgetStopsTheLoopAtTheTokenCeiling pins the check that MaxSteps cannot
// make: a round that returns a lot of tokens is stopped before the next one.
func TestBudgetStopsTheLoopAtTheTokenCeiling(t *testing.T) {
	comp := &tokenCompleter{input: 40, output: 20}
	r := newTestRunner(t, comp, &fakeExecutor{},
		WithBudget(Budget{MaxTokens: 100}),
	)
	s, turn := newBudgetTurn()

	err := r.Run(context.Background(), s, turn)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Run = %v, want ErrBudgetExceeded", err)
	}
	if turn.Status != TurnStatusFailed {
		t.Errorf("turn status = %s, want failed", turn.Status)
	}
	// Two rounds fit under 100 tokens (60, then 120 crosses it); the third
	// round must not start.
	if comp.calls != 2 {
		t.Fatalf("completer called %d times, want 2", comp.calls)
	}
}

func TestBudgetStopsTheLoopAtTheCostCeiling(t *testing.T) {
	comp := &tokenCompleter{input: 1000, output: 0}
	cost := func(in, _, _ int) float64 { return float64(in) / 10000 } // 0.10 per round
	r := newTestRunner(t, comp, &fakeExecutor{},
		WithBudget(Budget{MaxCostUSD: 0.15, Cost: cost}),
	)
	s, turn := newBudgetTurn()

	err := r.Run(context.Background(), s, turn)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Run = %v, want ErrBudgetExceeded", err)
	}
	if comp.calls != 2 {
		t.Fatalf("completer called %d times, want 2 (0.10 then 0.20 crosses 0.15)", comp.calls)
	}
}

// TestBudgetRefusesACostCeilingItCannotPrice pins fail-closed: a cost ceiling
// with no price would never trip, and an operator reading the spec would believe
// a limit was in force. The turn is refused before the first model call.
func TestBudgetRefusesACostCeilingItCannotPrice(t *testing.T) {
	comp := &tokenCompleter{input: 1, output: 1}
	r := newTestRunner(t, comp, &fakeExecutor{}, WithBudget(Budget{MaxCostUSD: 1}))
	s, turn := newBudgetTurn()

	err := r.Run(context.Background(), s, turn)
	if !errors.Is(err, ErrBudgetUnpriceable) {
		t.Fatalf("Run = %v, want ErrBudgetUnpriceable", err)
	}
	if comp.calls != 0 {
		t.Fatalf("completer called %d times, want 0", comp.calls)
	}
	if turn.Status != TurnStatusFailed {
		t.Errorf("turn status = %s, want failed", turn.Status)
	}
}

func TestBudgetStopsTheLoopAtTheWallClockCeiling(t *testing.T) {
	comp := &tokenCompleter{input: 1, output: 1}
	r := newTestRunner(t, comp, &fakeExecutor{}, WithBudget(Budget{MaxWallTime: time.Nanosecond}))
	s, turn := newBudgetTurn()

	err := r.Run(context.Background(), s, turn)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Run = %v, want ErrBudgetExceeded", err)
	}
	if comp.calls != 0 {
		t.Fatalf("completer called %d times, want 0", comp.calls)
	}
}

// TestZeroBudgetLeavesTheTurnUnbounded keeps the default: a zero Budget is what
// a workspace that published no ceilings asked for.
func TestZeroBudgetLeavesTheTurnUnbounded(t *testing.T) {
	comp := &tokenCompleter{input: 1_000_000, output: 1_000_000}
	r := newTestRunner(t, comp, &fakeExecutor{}, WithBudget(Budget{}))
	s, turn := newBudgetTurn()

	// The fake never stops asking for a tool, so the loop ends on MaxSteps —
	// not on a budget.
	err := r.Run(context.Background(), s, turn)
	if errors.Is(err, ErrBudgetExceeded) || errors.Is(err, ErrBudgetUnpriceable) {
		t.Fatalf("Run = %v, want the step ceiling, not a budget error", err)
	}
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("Run = %v, want ErrMaxStepsExceeded", err)
	}
}

// TestStreamBudgetStopsTheLoop pins that the streaming loop enforces the same
// ceilings as the non-streaming one. Two loops that disagree about a safety
// limit are a defect waiting for the first streaming client.
func TestStreamBudgetStopsTheLoopAtTheTokenCeiling(t *testing.T) {
	chunk := llmprovider.ToolCallChunk{Index: 0, ID: "c1", Name: "noop", ArgsJSON: "{}"}
	comp := &fakeStreamCompleter{
		steps: []streamStep{
			{events: []streamEvent{toolEv(chunk)}, resp: llmprovider.Response{InputTokens: 40, OutputTokens: 20}},
			{events: []streamEvent{toolEv(chunk)}, resp: llmprovider.Response{InputTokens: 40, OutputTokens: 20}},
			{events: []streamEvent{toolEv(chunk)}, resp: llmprovider.Response{InputTokens: 40, OutputTokens: 20}},
		},
	}
	r := newTestRunner(t, comp, &fakeExecutor{}, WithBudget(Budget{MaxTokens: 100}))
	sr, ok := r.(StreamRunner)
	if !ok {
		t.Fatal("runner does not implement StreamRunner")
	}
	s, turn := newBudgetTurn()

	err := sr.Stream(context.Background(), s, turn, &captureHandler{})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Stream = %v, want ErrBudgetExceeded", err)
	}
	if turn.Status != TurnStatusFailed {
		t.Errorf("turn status = %s, want failed", turn.Status)
	}
}

func TestHarnessSpecRejectsBadBudgets(t *testing.T) {
	cases := []struct {
		name  string
		spec  HarnessSpec
		field string
	}{
		{name: "negative tokens", spec: HarnessSpec{MaxTokens: -1}, field: "maxTokens"},
		{name: "nan cost", spec: HarnessSpec{MaxCostUSD: math.NaN()}, field: "maxCostUSD"},
		{name: "infinite cost", spec: HarnessSpec{MaxCostUSD: math.Inf(1)}, field: "maxCostUSD"},
		{name: "negative cost", spec: HarnessSpec{MaxCostUSD: -0.01}, field: "maxCostUSD"},
		{name: "negative wall time", spec: HarnessSpec{MaxWallTimeSec: -1}, field: "maxWallTimeSec"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := tc.spec
			spec.Version = HarnessSpecVersion
			spec.Name = "budget"
			var verr *SpecValidationError
			if err := Validate(spec); !errors.As(err, &verr) {
				t.Fatalf("Validate = %v, want a spec validation error", err)
			} else if !strings.Contains(verr.Error(), tc.field) {
				t.Fatalf("error = %v, want it to name %q", verr, tc.field)
			}
		})
	}
}

// TestSpecFingerprintCoversBudgets keeps recorded turns comparable: a spec that
// only changes a ceiling is a different harness, and its fingerprint must say so.
func TestSpecFingerprintCoversBudgets(t *testing.T) {
	base := HarnessSpec{Version: HarnessSpecVersion, Name: "p"}
	for _, variant := range []HarnessSpec{
		{Version: HarnessSpecVersion, Name: "p", MaxTokens: 1000},
		{Version: HarnessSpecVersion, Name: "p", MaxCostUSD: 0.5},
		{Version: HarnessSpecVersion, Name: "p", MaxWallTimeSec: 30},
	} {
		if variant.Fingerprint() == base.Fingerprint() {
			t.Errorf("fingerprint ignores %+v", variant)
		}
	}
}

func TestBudgetFromSpec(t *testing.T) {
	cost := func(int, int, int) float64 { return 0 }
	spec := HarnessSpec{MaxTokens: 500, MaxCostUSD: 0.25, MaxWallTimeSec: 90}
	got := BudgetFromSpec(spec, cost)
	if got.MaxTokens != 500 || got.MaxCostUSD != 0.25 || got.MaxWallTime != 90*time.Second {
		t.Fatalf("BudgetFromSpec = %+v, want the three ceilings copied", got)
	}
	if got.Cost == nil {
		t.Fatal("BudgetFromSpec dropped a price it was given")
	}

	// A cost ceiling over an unpriced model keeps Cost nil, and the runner
	// refuses the turn rather than running it uncapped.
	unpriced := BudgetFromSpec(HarnessSpec{MaxCostUSD: 0.25}, nil)
	if unpriced.Cost != nil {
		t.Fatal("BudgetFromSpec invented a price")
	}

	// Without a cost ceiling the price is not consulted at all.
	noCeiling := BudgetFromSpec(HarnessSpec{MaxTokens: 10}, nil)
	if noCeiling.Cost != nil {
		t.Fatal("BudgetFromSpec set Cost without a cost ceiling")
	}
}

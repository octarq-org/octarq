package api

import (
	"context"
	"testing"
)

func TestAIStatusNilContext(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)
	ctx := context.Background()
	if out, err := h.aiStatus(ctx, &AIStatusInput{Ctx: nil}); err != nil {
		t.Errorf("aiStatus with nil Ctx = %v, want no error", err)
	} else if out == nil {
		t.Error("aiStatus with nil Ctx returned no output")
	}
}

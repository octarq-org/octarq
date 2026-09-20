package builtin

import (
	"testing"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	plugins := Default()
	if len(plugins) != 6 {
		t.Fatalf("expected 6 plugins in Default(), got %d", len(plugins))
	}
	for i, p := range plugins {
		if p == nil {
			t.Errorf("plugin at index %d is nil", i)
		}
	}
}

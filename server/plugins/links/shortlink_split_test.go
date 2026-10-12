package links

import (
	"fmt"
	"testing"
)

func TestSplitAssign_Sticky(t *testing.T) {
	rules := RoutingRules{
		{Type: "split", Weight: 50, Target: "A"},
		{Type: "split", Weight: 50, Target: "B"},
	}
	fp := "some-fingerprint"
	linkID := uint(42)
	target1, variant1, ok1 := splitAssign(rules, fp, linkID)
	for i := 0; i < 100; i++ {
		target2, variant2, ok2 := splitAssign(rules, fp, linkID)
		if target1 != target2 || variant1 != variant2 || ok1 != ok2 {
			t.Errorf("stickiness failed on iteration %d", i)
		}
	}
}

func TestSplitAssign_Distribution(t *testing.T) {
	rules := RoutingRules{
		{Type: "split", Weight: 50, Target: "A"},
		{Type: "split", Weight: 50, Target: "B"},
	}
	counts := make(map[string]int)
	linkID := uint(42)
	for i := 0; i < 10000; i++ {
		fp := fmt.Sprintf("fp-%d", i)
		target, _, _ := splitAssign(rules, fp, linkID)
		counts[target]++
	}
	for _, target := range []string{"A", "B"} {
		pct := float64(counts[target]) / 10000.0
		if pct < 0.45 || pct > 0.55 {
			t.Errorf("target %s distribution out of bounds: %f", target, pct)
		}
	}
}

func TestSplitAssign_CrossLinkNoBias(t *testing.T) {
	rules := RoutingRules{
		{Type: "split", Weight: 50, Target: "A"},
		{Type: "split", Weight: 50, Target: "B"},
	}
	fp := "consistent-fingerprint"
	counts := make(map[string]int)
	for linkID := uint(1); linkID <= 10; linkID++ {
		target, _, _ := splitAssign(rules, fp, linkID)
		counts[target]++
	}
	if counts["A"] == 10 || counts["B"] == 10 {
		t.Errorf("cross-link bias: all 10 links fell into the same bucket for one fingerprint")
	}
}

func TestSplitAssign_Compatibility(t *testing.T) {
	rules := RoutingRules{
		{Type: "geo", Match: "CN", Target: "C"},
	}
	fp := "fp"
	linkID := uint(42)
	_, _, ok := splitAssign(rules, fp, linkID)
	if ok {
		t.Errorf("expected false when no split rules are present")
	}
}

func TestSplitAssign_Under100(t *testing.T) {
	rules := RoutingRules{
		{Type: "split", Weight: 10, Target: "A"},
	}
	counts := make(map[string]int)
	linkID := uint(42)
	for i := 0; i < 10000; i++ {
		fp := fmt.Sprintf("fp-%d", i)
		_, variant, _ := splitAssign(rules, fp, linkID)
		counts[variant]++
	}
	pctA := float64(counts["A"]) / 10000.0
	if pctA < 0.05 || pctA > 0.15 {
		t.Errorf("target A distribution out of bounds: %f", pctA)
	}
	pctDefault := float64(counts["control"]) / 10000.0
	if pctDefault < 0.85 || pctDefault > 0.95 {
		t.Errorf("default distribution out of bounds: %f", pctDefault)
	}
}

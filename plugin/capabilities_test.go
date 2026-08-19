package plugin

import (
	"context"
	"reflect"
	"testing"
)

type barePlugin struct{}

func (barePlugin) Name() string        { return "bare" }
func (barePlugin) Models() []any       { return nil }
func (barePlugin) Mount(Mux, *Context) {}

type richPlugin struct{ barePlugin }

func (richPlugin) Name() string          { return "rich" }
func (richPlugin) Start(context.Context) {}
func (richPlugin) Menus() []MenuItem     { return nil }
func (richPlugin) Describe() Info        { return Info{} }

// typoPlugin is the failure this diagnostic exists for: the author meant
// MenuProvider but wrote Menu() instead of Menus(). Nothing fails to build,
// nothing logs, the plugin simply never appears in the sidebar.
type typoPlugin struct{ barePlugin }

func (typoPlugin) Name() string     { return "typo" }
func (typoPlugin) Menu() []MenuItem { return nil }

func TestDetectedCapabilities(t *testing.T) {
	if got := DetectedCapabilities(barePlugin{}); len(got) != 0 {
		t.Errorf("bare plugin reported capabilities %v, want none", got)
	}
	if got, want := DetectedCapabilities(richPlugin{}), []string{"start", "menu", "describe"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rich plugin capabilities = %v, want %v", got, want)
	}
	if got := DetectedCapabilities(nil); got != nil {
		t.Errorf("DetectedCapabilities(nil) = %v, want nil", got)
	}
}

// TestTypoedOptionalInterfaceIsVisiblyAbsent is the point of the whole helper:
// the misspelled method must NOT show up as a detected capability, so the
// author comparing what they wrote against what the host saw gets an answer.
func TestTypoedOptionalInterfaceIsVisiblyAbsent(t *testing.T) {
	caps := DetectedCapabilities(typoPlugin{})
	for _, c := range caps {
		if c == "menu" {
			t.Fatal("typoPlugin reported the \"menu\" capability; detection would be claiming a capability the host will never invoke")
		}
	}
	if len(caps) != 0 {
		t.Fatalf("typoPlugin capabilities = %v, want none", caps)
	}
}

// LogDetectedCapabilities must not panic on either shape.
func TestLogDetectedCapabilitiesDoesNotPanic(t *testing.T) {
	LogDetectedCapabilities(nil)
	LogDetectedCapabilities(barePlugin{})
	LogDetectedCapabilities(richPlugin{})
}

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

func (richPlugin) Name() string               { return "rich" }
func (richPlugin) Start(context.Context)      {}
func (richPlugin) Stop(context.Context) error { return nil }
func (richPlugin) Menus() []MenuItem          { return nil }
func (richPlugin) Describe() Info             { return Info{} }

type typoPlugin struct{ barePlugin }

func (typoPlugin) Name() string     { return "typo" }
func (typoPlugin) Menu() []MenuItem { return nil }

func TestDetectedCapabilities(t *testing.T) {
	if got := DetectedCapabilities(barePlugin{}); len(got) != 0 {
		t.Errorf("bare plugin reported capabilities %v, want none", got)
	}
	if got, want := DetectedCapabilities(richPlugin{}), []string{"start", "stop", "menu", "describe"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rich plugin capabilities = %v, want %v", got, want)
	}
	if got := DetectedCapabilities(nil); got != nil {
		t.Errorf("DetectedCapabilities(nil) = %v, want nil", got)
	}
}

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

func TestLogDetectedCapabilitiesDoesNotPanic(t *testing.T) {
	LogDetectedCapabilities(nil)
	LogDetectedCapabilities(barePlugin{})
	LogDetectedCapabilities(richPlugin{})
}

package app

import (
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// fakePlugin is a minimal plugin.Plugin carrying only a name and models.
type fakePlugin struct {
	name   string
	models []any
}

func (f fakePlugin) Name() string                      { return f.name }
func (f fakePlugin) Models() []any                     { return f.models }
func (f fakePlugin) Mount(plugin.Mux, *plugin.Context) {}

var _ plugin.Plugin = fakePlugin{}

// mirrorSetting mirrors the core "settings" table with a local struct — the
// documented convention for plugins reading core tables without importing
// internal/models. Must never trip the preflight.
type mirrorSetting struct {
	ID    uint `gorm:"primaryKey"`
	Key   string
	Value string
}

func (mirrorSetting) TableName() string { return "settings" }

// widgetA and widgetB are two DIFFERENT struct types both claiming the
// non-core table "widgets" — the genuine ownership collision.
type widgetA struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func (widgetA) TableName() string { return "widgets" }

type widgetB struct {
	ID    uint `gorm:"primaryKey"`
	Color string
}

func (widgetB) TableName() string { return "widgets" }

// gadget has no TableName override; the naming strategy must derive "gadgets".
type gadget struct {
	ID uint `gorm:"primaryKey"`
}

func TestPreflightAllowsCoreTableMirror(t *testing.T) {
	p := fakePlugin{name: "billing", models: []any{&mirrorSetting{}}}
	if err := preflightTableCollisions(nil, []plugin.Plugin{p}); err != nil {
		t.Fatalf("mirroring a core table must be allowed, got: %v", err)
	}
}

func TestPreflightRejectsTwoPluginsOwningSameTable(t *testing.T) {
	a := fakePlugin{name: "alpha", models: []any{&widgetA{}}}
	b := fakePlugin{name: "beta", models: []any{&widgetB{}}}
	err := preflightTableCollisions(nil, []plugin.Plugin{a, b})
	if err == nil {
		t.Fatal("expected a collision error, got nil")
	}
	for _, want := range []string{`"widgets"`, `"alpha"`, `"beta"`, "widgetA", "widgetB"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %s; got: %v", want, err)
		}
	}
}

func TestPreflightRejectsOnePluginDeclaringSameTableTwice(t *testing.T) {
	p := fakePlugin{name: "alpha", models: []any{&widgetA{}, &widgetB{}}}
	if err := preflightTableCollisions(nil, []plugin.Plugin{p}); err == nil {
		t.Fatal("two different model types on one table within one plugin must fail")
	}
}

func TestPreflightAllowsSameTypeDeclaredTwice(t *testing.T) {
	a := fakePlugin{name: "alpha", models: []any{&widgetA{}}}
	b := fakePlugin{name: "beta", models: []any{&widgetA{}}} // same struct type — idempotent
	if err := preflightTableCollisions(nil, []plugin.Plugin{a, b}); err != nil {
		t.Fatalf("identical model types must not collide, got: %v", err)
	}
}

func TestPreflightDerivedTableNames(t *testing.T) {
	// No TableName override: two plugins each with their own distinct derived
	// table must pass; the same derived table from different types must fail.
	a := fakePlugin{name: "alpha", models: []any{&gadget{}}}
	if err := preflightTableCollisions(nil, []plugin.Plugin{a}); err != nil {
		t.Fatalf("single derived table must pass, got: %v", err)
	}
}

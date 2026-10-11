package help_test

import (
	"io/fs"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/help"
)

func TestHelpCharacterization_MetadataAndContracts(t *testing.T) {
	p := help.New()

	if p.Name() != "help" {
		t.Fatalf("expected module name 'help', got %q", p.Name())
	}

	info := p.Describe()
	if !info.Core {
		t.Errorf("expected Core to be true for help plugin")
	}
	if info.Title != "Help" {
		t.Errorf("expected Title 'Help', got %q", info.Title)
	}

	if models := p.Models(); models != nil {
		t.Errorf("help plugin should not declare any database models, got %v", models)
	}

	menus := p.Menus()
	if len(menus) != 1 || menus[0].Path != "/help" {
		t.Errorf("unexpected menus: %+v", menus)
	}

	// Verify HelpDocsFS
	docFS := p.HelpDocsFS()
	if docFS == nil {
		t.Fatal("HelpDocsFS returned nil")
	}
	if entries, err := fs.ReadDir(docFS, "docs"); err != nil || len(entries) == 0 {
		t.Errorf("expected non-empty docs dir in HelpDocsFS, err=%v", err)
	}
}

func TestHelpCharacterization_CompileTimeAssertions(t *testing.T) {
	var p any = (*help.Plugin)(nil)
	if _, ok := p.(plugin.Plugin); !ok {
		t.Errorf("Plugin must implement plugin.Plugin")
	}
	if _, ok := p.(plugin.Describer); !ok {
		t.Errorf("Plugin must implement plugin.Describer")
	}
	if _, ok := p.(plugin.MenuProvider); !ok {
		t.Errorf("Plugin must implement plugin.MenuProvider")
	}
	if _, ok := p.(plugin.HelpDocsFS); !ok {
		t.Errorf("Plugin must implement plugin.HelpDocsFS")
	}
}

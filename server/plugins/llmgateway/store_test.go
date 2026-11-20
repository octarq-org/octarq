package llmgateway

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func newTestStore(t *testing.T) (*Store, plugin.VaultService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Provider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(db), newTestVaultService(t, db)
}

func TestFirstProviderBecomesDefault(t *testing.T) {
	s, vs := newTestStore(t)
	ctx := context.Background()
	row, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "Claude", Backend: "claude", APIKey: "k"}, vs)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !row.IsDefault {
		t.Fatal("first provider is not the default")
	}
	active, err := s.ActiveProvider(ctx, 1)
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if active.ID != row.ID {
		t.Fatalf("active = %d, want %d", active.ID, row.ID)
	}
}

func TestSecondProviderIsNotDefault(t *testing.T) {
	s, vs := newTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "A", Backend: "claude", APIKey: "k"}, vs); err != nil {
		t.Fatalf("create A: %v", err)
	}
	second, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "B", Backend: "openai", APIKey: "k"}, vs)
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	if second.IsDefault {
		t.Fatal("second provider became the default")
	}
}

func TestSetDefaultIsExclusive(t *testing.T) {
	s, vs := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateProvider(ctx, 1, ProviderInput{Name: "A", Backend: "claude", APIKey: "k"}, vs)
	b, _ := s.CreateProvider(ctx, 1, ProviderInput{Name: "B", Backend: "openai", APIKey: "k"}, vs)
	if err := s.SetDefault(ctx, 1, b.ID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	rows, err := s.ListProviders(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var defaults int
	for _, row := range rows {
		if row.IsDefault {
			defaults++
			if row.ID != b.ID {
				t.Fatalf("default is %d, want %d", row.ID, b.ID)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("defaults = %d, want 1", defaults)
	}
	_ = a
}

func TestActiveProviderRefusesAmbiguity(t *testing.T) {
	s, vs := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateProvider(ctx, 1, ProviderInput{Name: "A", Backend: "claude", APIKey: "k"}, vs)
	if _, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "B", Backend: "openai", APIKey: "k"}, vs); err != nil {
		t.Fatalf("create B: %v", err)
	}

	if err := s.db.Model(&Provider{}).Where("org_id = ?", 1).Update("is_default", false).Error; err != nil {
		t.Fatalf("clear default: %v", err)
	}
	if _, err := s.ActiveProvider(ctx, 1); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("active = %v, want ErrNotConfigured", err)
	}
	_ = a
}

func TestNoProviderFailsClosed(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.ActiveProvider(context.Background(), 1); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("active = %v, want ErrNotConfigured", err)
	}
}

func TestPriceIsInstanceWide(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	got, err := s.Price(ctx, 1, "qwen-flash")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if got.Input <= 0 || got.Output <= 0 {
		t.Fatalf("price = %+v, want the catalog values", got)
	}

	if got.Reasoning != got.Output {
		t.Fatalf("reasoning = %v, want the output rate %v", got.Reasoning, got.Output)
	}
	other, err := s.Price(ctx, 2, "qwen-flash")
	if err != nil || other != got {
		t.Fatalf("other workspace price = %+v, %v, want %+v", other, err, got)
	}
}

func TestPriceLookupIsCaseInsensitiveAndAliased(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	want, err := s.Price(ctx, 1, "qwen-flash")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	for _, name := range []string{"Qwen-Flash", " qwen-flash ", "alibaba-qwen/qwen-flash"} {
		if got, err := s.Price(ctx, 1, name); err != nil || got != want {
			t.Fatalf("price(%q) = %+v, %v, want %+v", name, got, err, want)
		}
	}
}

func TestPriceCatalogRefusesWhatItCannotBill(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Price(ctx, 1, "no-such-model"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("unknown model = %v, want ErrModelUnpriced", err)
	}
	if _, err := s.Price(ctx, 1, ""); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("empty model = %v, want ErrModelUnpriced", err)
	}

	if _, err := s.Price(ctx, 1, "DetectDocumentText"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("unpriced model = %v, want ErrModelUnpriced", err)
	}
}

func TestPriceAliasRefusesAmbiguity(t *testing.T) {
	c, err := parsePriceCatalog([]byte(`{
		"model_count": 3,
		"models": [
			{"model_id": "vendor-a/same-name", "pricing": {"input": 1, "output": 2}},
			{"model_id": "vendor-b/same-name", "pricing": {"input": 3, "output": 4}},
			{"model_id": "vendor-a/unique", "pricing": {"input": 5, "output": 6}}
		]
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := c.price("same-name"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("ambiguous alias = %v, want ErrModelUnpriced", err)
	}
	if _, err := c.price("vendor-x/same-name"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("prefixed ambiguous alias = %v, want ErrModelUnpriced", err)
	}
	if got, err := c.price("vendor-a/unique"); err != nil || got.Input != 5 || got.Output != 6 {
		t.Fatalf("unique = %+v, %v", got, err)
	}
	if got, err := c.price("vendor-x/unique"); err != nil || got.Input != 5 {
		t.Fatalf("unique suffix = %+v, %v", got, err)
	}
}

func TestParsePriceCatalogRefusesCorruption(t *testing.T) {
	for _, raw := range []string{
		`{`,
		`{"model_count": 0, "models": []}`,
		`{"model_count": 2, "models": [{"model_id": "a", "pricing": {"input": 1, "output": 2}}]}`,
		`{"model_count": 2, "models": [{"model_id": "a"}, {"model_id": "a"}]}`,
		`{"model_count": 1, "models": [{"model_id": ""}]}`,
	} {
		if _, err := parsePriceCatalog([]byte(raw)); err == nil {
			t.Fatalf("parse %s = nil, want an error", raw)
		}
	}
}

func TestZeroOrgIsRefused(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.ListProviders(context.Background(), 0); err == nil {
		t.Fatal("org 0 listed providers")
	}
}

func TestValidateBackend(t *testing.T) {
	if err := validateBackend("claude"); err != nil {
		t.Fatalf("claude: %v", err)
	}
	if err := validateBackend(""); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("empty = %v", err)
	}
	if err := validateBackend("no-such-backend"); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("unknown = %v", err)
	}
}

func TestProviderVaultLifecycleAndGuards(t *testing.T) {
	s, vs := newTestStore(t)
	ctx := context.Background()

	if _, err := (*Store)(nil).ListProviders(ctx, 1); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("nil store ListProviders = %v", err)
	}

	if _, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "NoVault", Backend: "openai", APIKey: "sk-1"}, nil); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("nil vs CreateProvider = %v", err)
	}

	row, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "Ollama", Backend: "ollama", BaseURL: "http://127.0.0.1:11434"}, vs)
	if err != nil || row.VaultItemID != 0 {
		t.Fatalf("create keyless provider: row=%+v err=%v", row, err)
	}
	if _, err := s.UpdateProvider(ctx, 1, row.ID, ProviderInput{Name: "Ollama", Backend: "ollama", APIKey: "k1"}, nil); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("nil vs UpdateProvider = %v", err)
	}
	row, err = s.UpdateProvider(ctx, 1, row.ID, ProviderInput{Name: "Ollama2", Backend: "openai", APIKey: "sk-added"}, vs)
	if err != nil || row.VaultItemID == 0 {
		t.Fatalf("update adding key: row=%+v err=%v", row, err)
	}

	row, err = s.UpdateProvider(ctx, 1, row.ID, ProviderInput{Name: "OpenAI", Backend: "openai", APIKey: "sk-rotated"}, vs)
	if err != nil {
		t.Fatalf("update rotating key: %v", err)
	}

	if err := vs.DeleteSecret(ctx, 1, row.VaultItemID); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	row2, err := s.UpdateProvider(ctx, 1, row.ID, ProviderInput{Name: "OpenAI", Backend: "openai", APIKey: "sk-recreated"}, vs)
	if err != nil || row2.VaultItemID == 0 || row2.VaultItemID == row.VaultItemID {
		t.Fatalf("expected recreated vault item, row2=%+v err=%v", row2, err)
	}

	if err := s.DeleteProvider(ctx, 1, row2.ID, vs); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if _, err := vs.GetSecret(ctx, 1, row2.VaultItemID); !errors.Is(err, plugin.ErrSecretNotFound) {
		t.Fatalf("expected vault secret shredded after DeleteProvider, got %v", err)
	}
	if _, err := s.GetProvider(ctx, 1, row2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := s.DeleteProvider(ctx, 1, 99999, vs); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound deleting missing provider, got %v", err)
	}
}

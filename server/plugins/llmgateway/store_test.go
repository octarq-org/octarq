package llmgateway

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Provider{}, &ModelPrice{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(db)
}

func seal(t *testing.T) func(string) (string, error) {
	t.Helper()
	return func(s string) (string, error) {
		if s == "" {
			return "", errors.New("empty")
		}
		return "sealed:" + s, nil
	}
}

// TestFirstProviderBecomesDefault pins that a workspace with one provider needs
// no second step: the first row is the default, so a fresh workspace can chat.
func TestFirstProviderBecomesDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	row, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "Claude", Backend: "claude", APIKey: "k"}, seal(t))
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

// TestSecondProviderIsNotDefault pins that only the first row is default: a
// workspace with two providers must choose, not pick the newest.
func TestSecondProviderIsNotDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "A", Backend: "claude", APIKey: "k"}, seal(t)); err != nil {
		t.Fatalf("create A: %v", err)
	}
	second, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "B", Backend: "openai", APIKey: "k"}, seal(t))
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	if second.IsDefault {
		t.Fatal("second provider became the default")
	}
}

// TestSetDefaultIsExclusive pins the single-default invariant: setting one
// provider default clears the flag on the others.
func TestSetDefaultIsExclusive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateProvider(ctx, 1, ProviderInput{Name: "A", Backend: "claude", APIKey: "k"}, seal(t))
	b, _ := s.CreateProvider(ctx, 1, ProviderInput{Name: "B", Backend: "openai", APIKey: "k"}, seal(t))
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

// TestActiveProviderRefusesAmbiguity pins that two providers and no default is
// an error, not a coin flip.
func TestActiveProviderRefusesAmbiguity(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateProvider(ctx, 1, ProviderInput{Name: "A", Backend: "claude", APIKey: "k"}, seal(t))
	if _, err := s.CreateProvider(ctx, 1, ProviderInput{Name: "B", Backend: "openai", APIKey: "k"}, seal(t)); err != nil {
		t.Fatalf("create B: %v", err)
	}
	// Clear the default the first provider got on create.
	if err := s.db.Model(&Provider{}).Where("org_id = ?", 1).Update("is_default", false).Error; err != nil {
		t.Fatalf("clear default: %v", err)
	}
	if _, err := s.ActiveProvider(ctx, 1); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("active = %v, want ErrNotConfigured", err)
	}
	_ = a
}

// TestNoProviderFailsClosed pins the absence rule: a workspace with no provider
// refuses rather than falling back to instance credentials.
func TestNoProviderFailsClosed(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.ActiveProvider(context.Background(), 1); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("active = %v, want ErrNotConfigured", err)
	}
}

// TestPriceIsTenantScoped pins that one workspace's price never answers for
// another: prices are configuration, and a leak would mis-bill a tenant.
func TestPriceIsTenantScoped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.SetPrice(ctx, 1, ModelPrice{Model: "claude-opus-4-8", Input: 15, Output: 75, Reasoning: 30}); err != nil {
		t.Fatalf("set price: %v", err)
	}
	got, err := s.Price(ctx, 1, "claude-opus-4-8")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if got.Input != 15 || got.Output != 75 || got.Reasoning != 30 {
		t.Fatalf("price = %+v", got)
	}
	if _, err := s.Price(ctx, 2, "claude-opus-4-8"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("other tenant price = %v, want ErrModelUnpriced", err)
	}
}

// TestPriceUpsertKeepsOneRow pins that saving a price twice replaces it.
func TestPriceUpsertKeepsOneRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.SetPrice(ctx, 1, ModelPrice{Model: "m", Input: 1, Output: 2}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.SetPrice(ctx, 1, ModelPrice{Model: "m", Input: 3, Output: 4}); err != nil {
		t.Fatalf("set again: %v", err)
	}
	rows, err := s.ListPrices(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Input != 3 || rows[0].Output != 4 {
		t.Fatalf("row = %+v", rows[0])
	}
}

// TestPriceRejectsNegative pins that a negative price is refused: a negative
// cost would credit a budget instead of spending it.
func TestPriceRejectsNegative(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetPrice(context.Background(), 1, ModelPrice{Model: "m", Input: -1}); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("negative price = %v, want ErrInvalidProvider", err)
	}
}

// TestZeroOrgIsRefused pins that a call with no workspace cannot read rows.
func TestZeroOrgIsRefused(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.ListProviders(context.Background(), 0); err == nil {
		t.Fatal("org 0 listed providers")
	}
	if _, err := s.Price(context.Background(), 0, "m"); err == nil {
		t.Fatal("org 0 read a price")
	}
}

// TestValidateBackend pins that a typo fails at save time.
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

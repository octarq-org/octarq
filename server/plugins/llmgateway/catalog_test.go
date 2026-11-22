package llmgateway

import (
	"bytes"
	"compress/gzip"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func makeGzBundle(t *testing.T, rawJSON string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatalf("gzip writer: %v", err)
	}
	if _, err := gw.Write([]byte(rawJSON)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func TestCatalogParseValidation(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name:    "empty models",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[]}`,
			wantErr: true,
		},
		{
			name:    "negative input",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",-1,10,0,0,0,""]]}`,
			wantErr: true,
		},
		{
			name:    "null input",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",null,10,0,0,0,""]]}`,
			wantErr: true,
		},
		{
			name:    "negative output",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",1,-10,0,0,0,""]]}`,
			wantErr: true,
		},
		{
			name:    "null output",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",1,null,0,0,0,""]]}`,
			wantErr: true,
		},
		{
			name:    "null optional fields become zero",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",1,10,null,null,null,""]]}`,
			wantErr: false,
		},
		{
			name:    "short row",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",1,10,0,0,0]]}`,
			wantErr: true,
		},
		{
			name:    "negative cache read",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",1,10,-0.1,0,0,""]]}`,
			wantErr: true,
		},
		{
			name:    "negative cache write",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",1,10,0,-0.1,0,""]]}`,
			wantErr: true,
		},
		{
			name:    "negative reasoning",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",1,10,0,0,-0.1,""]]}`,
			wantErr: true,
		},
		{
			name:    "non-string name",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",1,10,0,0,0,5]]}`,
			wantErr: true,
		},
		{
			name:    "empty model id",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["","p",1,10,0,0,0,""]]}`,
			wantErr: true,
		},
		{
			name:    "non-numeric input",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p","x",10,0,0,0,""]]}`,
			wantErr: true,
		},
		{
			name:    "missing provider id",
			json:    `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","",1,10,0,0,0,""]]}`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gz := makeGzBundle(t, tc.json)
			cat, err := ParseBundleGz(gz)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			p, err := cat.PriceFor("p", "m")
			if err != nil {
				t.Fatalf("PriceFor: %v", err)
			}
			if p.CacheRead != 0 || p.CacheWrite != 0 || p.Reasoning != 0 {
				t.Fatalf("expected zeros for optional fields, got %+v", p)
			}
		})
	}
}

func TestCatalogLookups(t *testing.T) {
	raw := `{
		"generated_at": "2026-11-22T00:00:00Z",
		"source": "https://models.dev/api.json",
		"models": [
			["claude-opus-4-8", "anthropic", 5, 25, 0.5, 6.25, 0, "Claude Opus 4.8"],
			["deepseek-v4-pro", "deepseek", 0.66, 1.98, 0.022, 0, 1.98, "DeepSeek V4 Pro"],
			["deepseek/deepseek-v4-flash", "openrouter", 0.09, 0.18, 0.018, 0, 0, "DeepSeek V4 Flash"],
			["llama-3.3-70b-instruct", "provider-a", 1, 2, 0, 0, 0, "Llama A"],
			["llama-3.3-70b-instruct", "provider-b", 3, 4, 0, 0, 0, "Llama B"],
			["equal-model", "p1", 2, 4, 0.1, 0.2, 0, "Equal"],
			["equal-model", "p2", 2, 4, 0.1, 0.2, 0, "Equal"],
			["gpt-4o", "openai", 2.5, 10, 1.25, 0, 0, "GPT-4o"]
		]
	}`

	gz := makeGzBundle(t, raw)
	cat, err := ParseBundleGz(gz)
	if err != nil {
		t.Fatalf("parse bundle: %v", err)
	}

	t.Run("exact qualified match", func(t *testing.T) {
		p, err := cat.PriceFor("anthropic", "claude-opus-4-8")
		if err != nil {
			t.Fatalf("PriceFor: %v", err)
		}
		if p.Input != 5 || p.Output != 25 || p.CacheRead != 0.5 || p.CacheWrite != 6.25 {
			t.Fatalf("unexpected price: %+v", p)
		}
	})

	t.Run("dot dash equivalence", func(t *testing.T) {
		p, err := cat.PriceFor("anthropic", "claude-opus-4.8")
		if err != nil {
			t.Fatalf("PriceFor: %v", err)
		}
		if p.Input != 5 || p.Output != 25 {
			t.Fatalf("unexpected price: %+v", p)
		}
	})

	t.Run("snapshot stripping", func(t *testing.T) {
		p, err := cat.PriceFor("openai", "gpt-4o-2024-08-06")
		if err != nil {
			t.Fatalf("PriceFor: %v", err)
		}
		if p.Input != 2.5 || p.Output != 10 {
			t.Fatalf("unexpected price: %+v", p)
		}

		p2, err := cat.PriceFor("openai", "gpt-4o-20240806")
		if err != nil {
			t.Fatalf("PriceFor: %v", err)
		}
		if p2.Input != 2.5 || p2.Output != 10 {
			t.Fatalf("unexpected price: %+v", p2)
		}
	})

	t.Run("bare unique convergence", func(t *testing.T) {
		pid, mid, p, err := cat.ResolveBare("claude-opus-4-8")
		if err != nil {
			t.Fatalf("ResolveBare: %v", err)
		}
		if pid != "anthropic" || mid != "claude-opus-4-8" || p.Input != 5 {
			t.Fatalf("got %s %s %+v", pid, mid, p)
		}
	})

	t.Run("bare equal price convergence", func(t *testing.T) {
		_, mid, p, err := cat.ResolveBare("equal-model")
		if err != nil {
			t.Fatalf("ResolveBare: %v", err)
		}
		if mid != "equal-model" || p.Input != 2 || p.Output != 4 {
			t.Fatalf("got %s %+v", mid, p)
		}
	})

	t.Run("bare divergence is ambiguous", func(t *testing.T) {
		_, _, _, err := cat.ResolveBare("llama-3.3-70b-instruct")
		if !errors.Is(err, plugin.ErrModelAmbiguous) {
			t.Fatalf("expected ErrModelAmbiguous, got %v", err)
		}
	})

	t.Run("unpriced model", func(t *testing.T) {
		_, _, _, err := cat.ResolveBare("no-such-model")
		if !errors.Is(err, plugin.ErrModelUnpriced) {
			t.Fatalf("expected ErrModelUnpriced, got %v", err)
		}
		_, err = cat.PriceFor("anthropic", "no-such-model")
		if !errors.Is(err, plugin.ErrModelUnpriced) {
			t.Fatalf("expected ErrModelUnpriced, got %v", err)
		}
	})

	t.Run("known provider prefix resolve bare", func(t *testing.T) {
		pid, mid, p, err := cat.ResolveBare("openrouter/deepseek/deepseek-v4-flash")
		if err != nil {
			t.Fatalf("ResolveBare: %v", err)
		}
		if pid != "openrouter" || mid != "deepseek/deepseek-v4-flash" || p.Input != 0.09 {
			t.Fatalf("got %s %s %+v", pid, mid, p)
		}

		_, _, _, err = cat.ResolveBare("openrouter/no-such-model")
		if !errors.Is(err, plugin.ErrModelUnpriced) {
			t.Fatalf("expected ErrModelUnpriced, got %v", err)
		}
	})
}

func TestCatalogBillableUsesDisplayName(t *testing.T) {
	raw := `{
		"generated_at": "2026-11-22T00:00:00Z",
		"source": "https://models.dev/api.json",
		"models": [
			["named-model", "provider-a", 1, 2, 0.1, 0.2, 0, "Named Model"],
			["bare-model", "provider-b", 3, 4, 0, 0, 0, ""]
		]
	}`
	cat, err := ParseBundleGz(makeGzBundle(t, raw))
	if err != nil {
		t.Fatalf("parse bundle: %v", err)
	}
	if got := cat.ModelCount(); got != 2 {
		t.Fatalf("ModelCount = %d, want 2", got)
	}

	rows := cat.Billable()
	names := map[string]string{}
	for _, row := range rows {
		names[row.ProviderID+"/"+row.Model] = row.DisplayName
	}
	if names["provider-a/named-model"] != "Named Model" {
		t.Fatalf("displayName = %q, want Named Model", names["provider-a/named-model"])
	}
	if names["provider-b/bare-model"] != "bare-model" {
		t.Fatalf("displayName = %q, want the model id fallback", names["provider-b/bare-model"])
	}
}

func TestLoadCatalogRestoresTheSeed(t *testing.T) {
	setCatalogForTest(t, nil)
	if got := loadCatalog(); got == nil {
		t.Fatal("loadCatalog returned nil after the global catalog was cleared")
	}
}

func TestNilCatalogIsUnavailable(t *testing.T) {
	var c *Catalog
	if _, err := c.PriceFor("p", "m"); !errors.Is(err, plugin.ErrPricingUnavailable) {
		t.Fatalf("PriceFor = %v, want ErrPricingUnavailable", err)
	}
	if _, _, _, err := c.ResolveBare("m"); !errors.Is(err, plugin.ErrPricingUnavailable) {
		t.Fatalf("ResolveBare = %v, want ErrPricingUnavailable", err)
	}
	if c.Billable() != nil {
		t.Fatal("Billable on a nil catalog returned rows")
	}
	if c.ModelCount() != 0 {
		t.Fatalf("ModelCount = %d, want 0", c.ModelCount())
	}
}

func TestPriceForLowercasesTheProvider(t *testing.T) {
	raw := `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","anthropic",1,2,0,0,0,"M"]]}`
	cat, err := ParseBundleGz(makeGzBundle(t, raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := cat.PriceFor("Anthropic", "m"); err != nil {
		t.Fatalf("PriceFor = %v, want the lowercased provider to resolve", err)
	}
}

func TestPriceForRefusesEmptyArguments(t *testing.T) {
	raw := `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","p",1,2,0,0,0,"M"]]}`
	cat, err := ParseBundleGz(makeGzBundle(t, raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := cat.PriceFor("", "m"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("empty provider = %v, want ErrModelUnpriced", err)
	}
	if _, err := cat.PriceFor("p", "  "); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("empty model = %v, want ErrModelUnpriced", err)
	}
	if _, _, _, err := cat.ResolveBare(""); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("empty bare model = %v, want ErrModelUnpriced", err)
	}
}

func TestResolveBareRefusesAnUnknownQualifiedName(t *testing.T) {
	raw := `{"generated_at":"2026-11-22T00:00:00Z","source":"https://models.dev/api.json","models":[["m","anthropic",1,2,0,0,0,"M"]]}`
	cat, err := ParseBundleGz(makeGzBundle(t, raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, _, _, err := cat.ResolveBare("anthropic/"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("empty rest = %v, want ErrModelUnpriced", err)
	}
	if _, _, _, err := cat.ResolveBare("anthropic/no-such-model"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("unknown qualified name = %v, want ErrModelUnpriced", err)
	}
}

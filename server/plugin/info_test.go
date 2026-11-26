package plugin

import "testing"

func TestPluginCategoriesMatchValidCategories(t *testing.T) {
	cats := PluginCategories()
	if len(cats) != len(ValidCategories) {
		t.Fatalf("PluginCategories has %d entries, ValidCategories has %d", len(cats), len(ValidCategories))
	}
	for _, c := range cats {
		if !ValidCategories[c.Key] {
			t.Fatalf("category %q is not in ValidCategories", c.Key)
		}
		if c.Tone == "" {
			t.Fatalf("category %q has no tone", c.Key)
		}
		for _, lang := range []string{"en", "zh", "es", "pt", "ja"} {
			if c.Labels[lang] == "" {
				t.Fatalf("category %q has no %s label", c.Key, lang)
			}
		}
	}
}

func TestPluginCategoriesReturnsCopy(t *testing.T) {
	cats := PluginCategories()
	cats[0].Labels["en"] = "changed"
	if PluginCategories()[0].Labels["en"] == "changed" {
		t.Fatal("PluginCategories exposes a shared label map")
	}
}

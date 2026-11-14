package ontology

import (
	"testing"

	core "github.com/octarq-org/octarq/server/ontology"
)

// TestCatalogNamesTheTierThePageNeeds pins the value the generic admin page puts
// on its upsell mask. A declared Pro entity answers 402 on the first list, and
// without this field the mask can only say "Locked feature" — or label an
// elite-only entity as Pro, which is a worse lie than saying nothing.
func TestCatalogNamesTheTierThePageNeeds(t *testing.T) {
	catalogTier := func(t *testing.T, tiers map[core.Operation]string) string {
		t.Helper()
		spec := core.Spec{
			Slug:       "notes",
			Label:      "Note",
			Fields:     []core.Field{{Name: "body", Type: core.TypeText}},
			Operations: map[core.Operation]core.OperationACL{},
		}
		for _, op := range core.Operations {
			spec.Operations[op] = core.OperationACL{Roles: []string{"member"}, Tier: tiers[op]}
		}
		e := newEntity(spec)
		if err := e.validate(); err != nil {
			t.Fatalf("validate: %v", err)
		}
		return e.catalog(func(core.Operation) bool { return true }).Tier
	}

	for _, tc := range []struct {
		name string
		tier string
	}{
		{name: "free", tier: ""},
		{name: "pro", tier: core.TierPro},
		{name: "elite", tier: core.TierElite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			all := map[core.Operation]string{}
			for _, op := range core.Operations {
				all[op] = tc.tier
			}
			if got := catalogTier(t, all); got != tc.tier {
				t.Fatalf("catalog tier = %q, want %q", got, tc.tier)
			}
		})
	}

	// The page opens by listing, so listing is the operation whose tier decides
	// whether it can open at all. Taking the highest tier instead would gate a
	// readable page behind a write the caller never asked for.
	t.Run("list decides, not the highest operation", func(t *testing.T) {
		tiers := map[core.Operation]string{
			core.OpList:   core.TierPro,
			core.OpGet:    core.TierPro,
			core.OpCreate: core.TierElite,
			core.OpUpdate: core.TierElite,
			core.OpDelete: core.TierElite,
		}
		if got := catalogTier(t, tiers); got != core.TierPro {
			t.Fatalf("catalog tier = %q, want %q from the list operation", got, core.TierPro)
		}
	})
}

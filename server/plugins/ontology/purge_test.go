package ontology

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

// TestPurgeRemovesOnlyTheWorkspaceItWasGiven pins the seam the platform calls
// before it deletes an org. A generated table is nobody else's to clean up: with
// no service under plugin.PurgeServiceName("ontology"), every deleted workspace
// leaves its declared rows behind, silently.
func TestPurgeRemovesOnlyTheWorkspaceItWasGiven(t *testing.T) {
	db := openDocDB(t, ticketDoc)
	t.Setenv(EntitiesEnv, ticketDoc)
	t.Setenv(EntitiesFileEnv, "")

	provided := map[string]any{}
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Purge", "1.0"))
	p := New()
	p.Mount(nil, &plugin.Context{
		Huma:    api,
		DB:      db,
		Host:    &plugin.TestHost{},
		Provide: func(name string, service any) { provided[name] = service },
	})
	if err := p.Validate(context.Background(), nil); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	purge, ok := provided[plugin.PurgeServiceName("ontology")].(plugin.PurgeFunc)
	if !ok {
		t.Fatalf("no plugin.PurgeFunc under %q; a deleted workspace would keep its declared rows",
			plugin.PurgeServiceName("ontology"))
	}

	// One row per workspace, written the way the plugin writes them.
	e := ticketEntity(t)
	table := core.TableName("tickets")
	for _, org := range []uint{1, 2} {
		tdb, err := plugin.NewTenantDB(db, org)
		if err != nil {
			t.Fatalf("tenant db for org %d: %v", org, err)
		}
		title := "ticket-" + strconv.Itoa(int(org))
		if _, err := (store{}).create(context.Background(), tdb, e, map[string]any{"title": title}); err != nil {
			t.Fatalf("create for org %d: %v", org, err)
		}
	}

	if err := purge(1); err != nil {
		t.Fatalf("purge(1): %v", err)
	}

	for _, want := range []struct {
		org  uint
		rows int64
	}{{1, 0}, {2, 1}} {
		var got int64
		if err := db.Table(table).Where("org_id = ?", want.org).Count(&got).Error; err != nil {
			t.Fatalf("count org %d: %v", want.org, err)
		}
		if got != want.rows {
			t.Fatalf("org %d has %d rows after purging org 1, want %d", want.org, got, want.rows)
		}
	}

	// No workspace is not "delete nothing and report success".
	if err := purge(0); err == nil {
		t.Fatal("purge(0) = nil, want a refusal")
	}
}

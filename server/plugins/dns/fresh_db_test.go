package dns

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// memDBCounter keeps in-memory DSNs unique across every invocation, including
// -count=N reruns: the shared helpers key their DSN on t.Name(), which is
// identical across reruns and so collides on the second run. Tests here must
// not depend on prior runs' rows.
var memDBCounter atomic.Uint64

// freshMemDB opens a uniquely-named shared in-memory sqlite database.
func freshMemDB(t *testing.T) *gorm.DB {
	t.Helper()
	n := memDBCounter.Add(1)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:memdb-%d?mode=memory&cache=shared", n)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Domain{}, &ProviderAccount{}, &DDNSToken{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// setupFreshTestDB mirrors setupFullDNSTestDB's wiring (org id from the
// X-Org-ID header, default 1; role gate refuses X-Role=member) but backs the
// plugin with a fresh, uniquely-named database.
func setupFreshTestDB(t *testing.T) (*Plugin, func(req *http.Request) huma.Context) {
	t.Helper()
	p := New()
	p.db = freshMemDB(t)
	p.audit = func(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {}
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
	p.orgID = func(r *http.Request) uint {
		if val := r.Header.Get("X-Org-ID"); val != "" {
			var id uint
			fmt.Sscanf(val, "%d", &id)
			return id
		}
		return 1
	}
	p.requireRole = func(r *http.Request, role string) bool {
		return r.Header.Get("X-Role") != "member"
	}
	mkCtx := func(r *http.Request) huma.Context {
		if r.Header.Get("X-Org-ID") == "" {
			r.Header.Set("X-Org-ID", "1")
		}
		return humago.NewContext(nil, r, httptest.NewRecorder())
	}
	return p, mkCtx
}

// setupFreshGateTestDB mirrors setupGateTestDB's wiring (role string passed
// per context) with a fresh database.
func setupFreshGateTestDB(t *testing.T) (*Plugin, func(role string) huma.Context) {
	t.Helper()
	p := New()
	p.db = freshMemDB(t)
	p.audit = func(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {}
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
	p.orgID = func(r *http.Request) uint {
		var id uint
		fmt.Sscanf(r.Header.Get("X-Org-ID"), "%d", &id)
		if id == 0 {
			id = 1
		}
		return id
	}
	p.requireRole = func(r *http.Request, role string) bool {
		return r.Header.Get("X-Role") == "admin"
	}
	mkCtx := func(role string) huma.Context {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Header.Set("X-Org-ID", "1")
		r.Header.Set("X-Role", role)
		return humago.NewContext(nil, r, httptest.NewRecorder())
	}
	return p, mkCtx
}

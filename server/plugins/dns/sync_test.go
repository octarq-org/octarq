package dns

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var dbCounter int64

func freshBenchDB(b *testing.B) *gorm.DB {
	b.Helper()
	n := atomic.AddInt64(&dbCounter, 1)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:benchdb-%d?mode=memory&cache=shared", n)), &gorm.Config{})
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Domain{}, &ProviderAccount{}, &DDNSToken{})...); err != nil {
		b.Fatalf("migrate: %v", err)
	}
	return db
}

func BenchmarkSyncDomains(b *testing.B) {
	p := New()
	p.db = freshBenchDB(b)
	p.audit = func(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {}
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
	p.orgID = func(r *http.Request) uint { return 1 }
	p.requireRole = func(r *http.Request, role string) bool { return true }

	fake := &fakeDNSProvider{zones: make([]dnsprovider.Zone, 0, 1000)}
	for i := 0; i < 1000; i++ {
		fake.zones = append(fake.zones, dnsprovider.Zone{ID: fmt.Sprintf("z-%d", i), Name: fmt.Sprintf("bench%d.com", i)})
	}
	provName := "sync-bench-prov"
	dnsprovider.Register(provName, func(config []byte) (dnsprovider.Provider, error) {
		return fake, nil
	})

	acc := &ProviderAccount{OrgID: 1, Name: "bench", Type: provName, Config: "{}"}
	p.db.Create(acc)

	for i := 0; i < 500; i++ {
		p.db.Create(&Domain{OrgID: 1, Name: fmt.Sprintf("bench%d.com", i), ProviderAccountID: acc.ID, ZoneID: "old-z"})
	}

	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("x-octarq-user", `{"id":1,"role":"admin"}`)

	mkCtx := func(req *http.Request) huma.Context {
		return humago.NewContext(nil, req, httptest.NewRecorder())
	}
	ctx := mkCtx(req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		p.db.Model(&Domain{}).Where("1 = 1").Update("zone_id", "old-z")
		p.db.Where("name >= 'bench500.com'").Delete(&Domain{})
		b.StartTimer()

		_, err := p.syncDomains(context.Background(), &SyncDomainsInput{
			Ctx: ctx,
			Body: struct {
				ProviderAccountID uint `json:"providerAccountId,omitempty"`
			}{ProviderAccountID: acc.ID},
		})
		if err != nil {
			b.Fatalf("syncDomains: %v", err)
		}
	}
}

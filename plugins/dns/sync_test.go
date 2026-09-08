package dns

import (
	"context"
	"fmt"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBenchmarkSyncDomains(t *testing.T) {
	p, mkCtx := setupFreshTestDB(t)

	numZones := 500
	fake := &fakeDNSProvider{
		zones: make([]dnsprovider.Zone, numZones),
	}
	for i := 0; i < numZones; i++ {
		fake.zones[i] = dnsprovider.Zone{ID: fmt.Sprintf("z-%d", i), Name: fmt.Sprintf("bench%d.com", i)}
	}

	provName := registerFakeProvider(t, "sync-bench-prov", fake)
	acc := ProviderAccount{OrgID: 1, Name: "bench-acc", Type: provName, Config: "{}"}
	p.db.Create(&acc)

	req := httptest.NewRequest(http.MethodPost, "/api/domains/sync", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")

	in := &SyncDomainsInput{
		Ctx: mkCtx(req),
		Body: struct {
			ProviderAccountID uint `json:"providerAccountId,omitempty"`
		}{acc.ID},
	}

	// Create N times to measure average

	// we will run benchmark inside the test
	result := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p.db.Exec("DELETE FROM domains WHERE owner_id = 1")
			_, err := p.syncDomains(context.Background(), in)
			if err != nil {
				b.Fatalf("syncDomains failed: %v", err)
			}
		}
	})
	fmt.Printf("BenchmarkSyncDomains (500 zones): %v\n", result)
}

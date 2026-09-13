// Package dnsprovider abstracts DNS record management across providers.
//
// Cloudflare and DNSPod are implemented today; the Provider interface and the
// registry leave room for further backends (e.g. Aliyun / Route53) without
// touching callers. Each record carries a Comment field — that is where a
// record's "note" lives, mapping onto Cloudflare's native per-record comment.
package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Record is a provider-agnostic DNS record.
type Record struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"` // A, AAAA, CNAME, TXT, MX, ...
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied,omitempty"`
	Comment  string `json:"comment,omitempty"` // the per-record note
	Priority *int   `json:"priority,omitempty"`
}

// Zone is a DNS zone (domain) visible to the credentials.
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Provider is the DNS backend contract.
type Provider interface {
	// ListZones returns every zone (domain) the credentials can manage.
	ListZones(ctx context.Context) ([]Zone, error)
	ListRecords(ctx context.Context, zoneID string) ([]Record, error)
	CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error)
	UpdateRecord(ctx context.Context, zoneID string, r Record) (Record, error)
	DeleteRecord(ctx context.Context, zoneID, recordID string) error
	// VerifyZone confirms the credentials can access the zone and returns its name.
	VerifyZone(ctx context.Context, zoneID string) (string, error)
}

// Factory builds a Provider from a decrypted JSON credentials blob.
type Factory func(credsJSON []byte) (Provider, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register makes a provider available by name.
func Register(name string, f Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = f
}

// New constructs a provider by name from its credentials JSON.
func New(name string, credsJSON []byte) (Provider, error) {
	registryMu.RLock()
	f, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown dns provider %q", name)
	}
	return f(credsJSON)
}

// Names returns a sorted list of registered provider names.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MarshalCreds is a small helper for callers building credential blobs.
func MarshalCreds(v any) ([]byte, error) { return json.Marshal(v) }

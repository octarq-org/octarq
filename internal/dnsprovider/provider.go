// Package dnsprovider abstracts DNS record management across providers.
//
// Cloudflare is implemented today; the Provider interface and the registry
// keep room for Aliyun / DNSPod / Route53 without touching callers. Each
// record carries a Comment field — that is where a record's "note" lives,
// mapping onto Cloudflare's native per-record comment.
package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
)

// Record is a provider-agnostic DNS record.
type Record struct {
	ID      string `json:"id"`
	Type    string `json:"type"` // A, AAAA, CNAME, TXT, MX, ...
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment"` // the per-record note
	Priority *int  `json:"priority,omitempty"`
}

// Provider is the DNS backend contract.
type Provider interface {
	ListRecords(ctx context.Context, zoneID string) ([]Record, error)
	CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error)
	UpdateRecord(ctx context.Context, zoneID string, r Record) (Record, error)
	DeleteRecord(ctx context.Context, zoneID, recordID string) error
	// VerifyZone confirms the credentials can access the zone and returns its name.
	VerifyZone(ctx context.Context, zoneID string) (string, error)
}

// Factory builds a Provider from a decrypted JSON credentials blob.
type Factory func(credsJSON []byte) (Provider, error)

var registry = map[string]Factory{}

// Register makes a provider available by name.
func Register(name string, f Factory) { registry[name] = f }

// New constructs a provider by name from its credentials JSON.
func New(name string, credsJSON []byte) (Provider, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown dns provider %q", name)
	}
	return f(credsJSON)
}

// Names returns the registered provider names.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

// MarshalCreds is a small helper for callers building credential blobs.
func MarshalCreds(v any) ([]byte, error) { return json.Marshal(v) }

package dnsprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const cfAPIBase = "https://api.cloudflare.com/client/v4"

func init() {
	Register("cloudflare", func(creds []byte) (Provider, error) {
		var c cfCreds
		if err := json.Unmarshal(creds, &c); err != nil {
			return nil, fmt.Errorf("parse cloudflare creds: %w", err)
		}
		if c.APIToken == "" {
			return nil, fmt.Errorf("cloudflare: apiToken required")
		}
		return &Cloudflare{token: c.APIToken, hc: &http.Client{Timeout: 15 * time.Second}}, nil
	})
}

type cfCreds struct {
	APIToken string `json:"apiToken"`
}

// Cloudflare implements Provider against the Cloudflare API v4.
type Cloudflare struct {
	token string
	hc    *http.Client
}

type cfResp struct {
	Success bool            `json:"success"`
	Errors  []cfError       `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e cfError) String() string { return fmt.Sprintf("%d: %s", e.Code, e.Message) }

func (c *Cloudflare) do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, cfAPIBase+path, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out cfResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("cloudflare: decode response: %w", err)
	}
	if !out.Success {
		if len(out.Errors) > 0 {
			return nil, fmt.Errorf("cloudflare: %s", out.Errors[0].String())
		}
		return nil, fmt.Errorf("cloudflare: request failed (HTTP %d)", resp.StatusCode)
	}
	return out.Result, nil
}

type cfRecord struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl,omitempty"`
	Proxied  bool   `json:"proxied"`
	Comment  string `json:"comment"`
	Priority *int   `json:"priority,omitempty"`
}

func toCF(r Record) cfRecord {
	ttl := r.TTL
	if ttl == 0 {
		ttl = 1 // 1 = automatic in Cloudflare
	}
	return cfRecord{
		ID: r.ID, Type: r.Type, Name: r.Name, Content: r.Content,
		TTL: ttl, Proxied: r.Proxied, Comment: r.Comment, Priority: r.Priority,
	}
}

func fromCF(r cfRecord) Record {
	return Record{
		ID: r.ID, Type: r.Type, Name: r.Name, Content: r.Content,
		TTL: r.TTL, Proxied: r.Proxied, Comment: r.Comment, Priority: r.Priority,
	}
}

func (c *Cloudflare) ListRecords(ctx context.Context, zoneID string) ([]Record, error) {
	raw, err := c.do(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records?per_page=500", nil)
	if err != nil {
		return nil, err
	}
	var recs []cfRecord
	if err := json.Unmarshal(raw, &recs); err != nil {
		return nil, err
	}
	out := make([]Record, len(recs))
	for i, r := range recs {
		out[i] = fromCF(r)
	}
	return out, nil
}

func (c *Cloudflare) CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error) {
	raw, err := c.do(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", toCF(r))
	if err != nil {
		return Record{}, err
	}
	var out cfRecord
	if err := json.Unmarshal(raw, &out); err != nil {
		return Record{}, err
	}
	return fromCF(out), nil
}

func (c *Cloudflare) UpdateRecord(ctx context.Context, zoneID string, r Record) (Record, error) {
	raw, err := c.do(ctx, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+r.ID, toCF(r))
	if err != nil {
		return Record{}, err
	}
	var out cfRecord
	if err := json.Unmarshal(raw, &out); err != nil {
		return Record{}, err
	}
	return fromCF(out), nil
}

func (c *Cloudflare) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	_, err := c.do(ctx, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+recordID, nil)
	return err
}

func (c *Cloudflare) VerifyZone(ctx context.Context, zoneID string) (string, error) {
	raw, err := c.do(ctx, http.MethodGet, "/zones/"+zoneID, nil)
	if err != nil {
		return "", err
	}
	var z struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &z); err != nil {
		return "", err
	}
	return z.Name, nil
}

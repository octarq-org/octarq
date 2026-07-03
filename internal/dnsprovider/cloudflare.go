package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
)

func init() {
	Register("cloudflare", func(creds []byte) (Provider, error) {
		var c cfCreds
		if err := json.Unmarshal(creds, &c); err != nil {
			return nil, fmt.Errorf("parse cloudflare creds: %w", err)
		}
		if c.APIToken == "" {
			return nil, fmt.Errorf("cloudflare: apiToken required")
		}
		api, err := cloudflare.NewWithAPIToken(c.APIToken)
		if err != nil {
			return nil, fmt.Errorf("cloudflare: initialize api client: %w", err)
		}
		return &Cloudflare{api: api}, nil
	})
}

type cfCreds struct {
	APIToken string `json:"apiToken"`
}

// Cloudflare implements Provider using the official Cloudflare Go SDK.
type Cloudflare struct {
	api *cloudflare.API
}

func (c *Cloudflare) ListZones(ctx context.Context) ([]Zone, error) {
	var zones []Zone
	cfZones, err := c.api.ListZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: list zones: %w", err)
	}
	for _, z := range cfZones {
		zones = append(zones, Zone{ID: z.ID, Name: z.Name})
	}
	return zones, nil
}

func (c *Cloudflare) ListRecords(ctx context.Context, zoneID string) ([]Record, error) {
	rc := cloudflare.ZoneIdentifier(zoneID)
	var records []Record
	params := cloudflare.ListDNSRecordsParams{}
	params.PerPage = 100

	for {
		cfRecs, resultInfo, err := c.api.ListDNSRecords(ctx, rc, params)
		if err != nil {
			return nil, fmt.Errorf("cloudflare: list records: %w", err)
		}
		for _, r := range cfRecs {
			var priority *int
			if r.Priority != nil {
				pVal := int(*r.Priority)
				priority = &pVal
			}
			proxied := false
			if r.Proxied != nil {
				proxied = *r.Proxied
			}
			records = append(records, Record{
				ID:       r.ID,
				Type:     r.Type,
				Name:     r.Name,
				Content:  r.Content,
				TTL:      r.TTL,
				Proxied:  proxied,
				Comment:  r.Comment,
				Priority: priority,
			})
		}
		if resultInfo == nil || resultInfo.Page >= resultInfo.TotalPages || len(cfRecs) == 0 {
			break
		}
		params.Page = resultInfo.Page + 1
	}
	return records, nil
}

func (c *Cloudflare) CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error) {
	rc := cloudflare.ZoneIdentifier(zoneID)

	var priority *uint16
	if r.Priority != nil {
		pVal := uint16(*r.Priority)
		priority = &pVal
	}

	params := cloudflare.CreateDNSRecordParams{
		Type:     r.Type,
		Name:     r.Name,
		Content:  r.Content,
		TTL:      r.TTL,
		Proxied:  &r.Proxied,
		Priority: priority,
		Comment:  r.Comment,
	}

	cfRec, err := c.api.CreateDNSRecord(ctx, rc, params)
	if err != nil {
		return Record{}, fmt.Errorf("cloudflare: create record: %w", err)
	}

	var resPriority *int
	if cfRec.Priority != nil {
		pVal := int(*cfRec.Priority)
		resPriority = &pVal
	}
	proxied := false
	if cfRec.Proxied != nil {
		proxied = *cfRec.Proxied
	}

	return Record{
		ID:       cfRec.ID,
		Type:     cfRec.Type,
		Name:     cfRec.Name,
		Content:  cfRec.Content,
		TTL:      cfRec.TTL,
		Proxied:  proxied,
		Comment:  cfRec.Comment,
		Priority: resPriority,
	}, nil
}

func (c *Cloudflare) UpdateRecord(ctx context.Context, zoneID string, r Record) (Record, error) {
	rc := cloudflare.ZoneIdentifier(zoneID)

	var priority *uint16
	if r.Priority != nil {
		pVal := uint16(*r.Priority)
		priority = &pVal
	}

	params := cloudflare.UpdateDNSRecordParams{
		ID:       r.ID,
		Type:     r.Type,
		Name:     r.Name,
		Content:  r.Content,
		TTL:      r.TTL,
		Proxied:  &r.Proxied,
		Priority: priority,
		Comment:  &r.Comment,
	}

	cfRec, err := c.api.UpdateDNSRecord(ctx, rc, params)
	if err != nil {
		return Record{}, fmt.Errorf("cloudflare: update record: %w", err)
	}

	var resPriority *int
	if cfRec.Priority != nil {
		pVal := int(*cfRec.Priority)
		resPriority = &pVal
	}
	proxied := false
	if cfRec.Proxied != nil {
		proxied = *cfRec.Proxied
	}

	return Record{
		ID:       cfRec.ID,
		Type:     cfRec.Type,
		Name:     cfRec.Name,
		Content:  cfRec.Content,
		TTL:      cfRec.TTL,
		Proxied:  proxied,
		Comment:  cfRec.Comment,
		Priority: resPriority,
	}, nil
}

func (c *Cloudflare) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	rc := cloudflare.ZoneIdentifier(zoneID)
	err := c.api.DeleteDNSRecord(ctx, rc, recordID)
	if err != nil {
		return fmt.Errorf("cloudflare: delete record: %w", err)
	}
	return nil
}

func (c *Cloudflare) VerifyZone(ctx context.Context, zoneID string) (string, error) {
	zone, err := c.api.ZoneDetails(ctx, zoneID)
	if err != nil {
		return "", fmt.Errorf("cloudflare: verify zone: %w", err)
	}
	return zone.Name, nil
}

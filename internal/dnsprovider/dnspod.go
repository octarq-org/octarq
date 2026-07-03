package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	dnspod "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

func init() {
	Register("dnspod", func(creds []byte) (Provider, error) {
		var c dpCreds
		if err := json.Unmarshal(creds, &c); err != nil {
			return nil, fmt.Errorf("parse dnspod creds: %w", err)
		}

		var secretID, secretKey string
		if c.SecretID != "" && c.SecretKey != "" {
			secretID = c.SecretID
			secretKey = c.SecretKey
		} else if c.Token != "" {
			parts := strings.Split(c.Token, ",")
			if len(parts) == 2 {
				secretID = parts[0]
				secretKey = parts[1]
			}
		}

		if secretID == "" || secretKey == "" {
			return nil, fmt.Errorf("dnspod: secretId and secretKey (or legacy token) required")
		}

		credential := common.NewCredential(secretID, secretKey)
		cpf := profile.NewClientProfile()
		client, err := dnspod.NewClient(credential, "", cpf)
		if err != nil {
			return nil, fmt.Errorf("dnspod: initialize client: %w", err)
		}
		return &DNSPod{client: client}, nil
	})
}

type dpCreds struct {
	Token     string `json:"token"`    // legacy: "ID,TOKEN"
	SecretID  string `json:"secretId"` // alternative split form
	SecretKey string `json:"secretKey"`
}

// DNSPod implements Provider using the official Tencent Cloud DNSPod Go SDK.
type DNSPod struct {
	client *dnspod.Client
}

func (d *DNSPod) ListZones(ctx context.Context) ([]Zone, error) {
	req := dnspod.NewDescribeDomainListRequest()
	resp, err := d.client.DescribeDomainListWithContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("dnspod: list domains: %w", err)
	}
	var zones []Zone
	if resp.Response != nil {
		for _, dm := range resp.Response.DomainList {
			if dm.DomainId != nil && dm.Name != nil {
				zones = append(zones, Zone{
					ID:   strconv.FormatUint(*dm.DomainId, 10),
					Name: *dm.Name,
				})
			}
		}
	}
	return zones, nil
}

func (d *DNSPod) ListRecords(ctx context.Context, zoneID string) ([]Record, error) {
	req := dnspod.NewDescribeRecordListRequest()
	zID, err := strconv.ParseUint(zoneID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("dnspod: invalid zoneID %q: %w", zoneID, err)
	}
	req.DomainId = &zID

	limit := uint64(3000)
	req.Limit = &limit

	resp, err := d.client.DescribeRecordListWithContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("dnspod: list records: %w", err)
	}

	var records []Record
	if resp.Response != nil {
		for _, r := range resp.Response.RecordList {
			if r.RecordId == nil || r.Type == nil || r.Name == nil || r.Value == nil {
				continue
			}

			var priority *int
			if r.MX != nil && *r.MX > 0 {
				pVal := int(*r.MX)
				priority = &pVal
			}

			ttl := 600
			if r.TTL != nil {
				ttl = int(*r.TTL)
			}

			records = append(records, Record{
				ID:       strconv.FormatUint(*r.RecordId, 10),
				Type:     *r.Type,
				Name:     *r.Name,
				Content:  *r.Value,
				TTL:      ttl,
				Priority: priority,
				Comment:  getPtrString(r.Remark),
			})
		}
	}
	return records, nil
}

func (d *DNSPod) CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error) {
	req := dnspod.NewCreateRecordRequest()
	zID, err := strconv.ParseUint(zoneID, 10, 64)
	if err != nil {
		return Record{}, fmt.Errorf("dnspod: invalid zoneID %q: %w", zoneID, err)
	}
	req.DomainId = &zID
	req.RecordType = &r.Type
	req.SubDomain = &r.Name
	req.Value = &r.Content

	ttlVal := uint64(r.TTL)
	if ttlVal > 0 {
		req.TTL = &ttlVal
	}

	if r.Priority != nil {
		mxVal := uint64(*r.Priority)
		req.MX = &mxVal
	}
	if r.Comment != "" {
		req.Remark = &r.Comment
	}

	line := "默认"
	req.RecordLine = &line

	resp, err := d.client.CreateRecordWithContext(ctx, req)
	if err != nil {
		return Record{}, fmt.Errorf("dnspod: create record: %w", err)
	}

	if resp.Response == nil || resp.Response.RecordId == nil {
		return Record{}, fmt.Errorf("dnspod: create record returned empty response")
	}

	r.ID = strconv.FormatUint(*resp.Response.RecordId, 10)
	return r, nil
}

func (d *DNSPod) UpdateRecord(ctx context.Context, zoneID string, r Record) (Record, error) {
	req := dnspod.NewModifyRecordRequest()
	zID, err := strconv.ParseUint(zoneID, 10, 64)
	if err != nil {
		return Record{}, fmt.Errorf("dnspod: invalid zoneID %q: %w", zoneID, err)
	}
	req.DomainId = &zID

	rID, err := strconv.ParseUint(r.ID, 10, 64)
	if err != nil {
		return Record{}, fmt.Errorf("dnspod: invalid recordID %q: %w", r.ID, err)
	}
	req.RecordId = &rID
	req.RecordType = &r.Type
	req.SubDomain = &r.Name
	req.Value = &r.Content

	ttlVal := uint64(r.TTL)
	if ttlVal > 0 {
		req.TTL = &ttlVal
	}

	if r.Priority != nil {
		mxVal := uint64(*r.Priority)
		req.MX = &mxVal
	}
	if r.Comment != "" {
		req.Remark = &r.Comment
	}

	line := "默认"
	req.RecordLine = &line

	_, err = d.client.ModifyRecordWithContext(ctx, req)
	if err != nil {
		return Record{}, fmt.Errorf("dnspod: update record: %w", err)
	}

	return r, nil
}

func (d *DNSPod) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	req := dnspod.NewDeleteRecordRequest()
	zID, err := strconv.ParseUint(zoneID, 10, 64)
	if err != nil {
		return fmt.Errorf("dnspod: invalid zoneID %q: %w", zoneID, err)
	}
	req.DomainId = &zID

	rID, err := strconv.ParseUint(recordID, 10, 64)
	if err != nil {
		return fmt.Errorf("dnspod: invalid recordID %q: %w", recordID, err)
	}
	req.RecordId = &rID

	_, err = d.client.DeleteRecordWithContext(ctx, req)
	if err != nil {
		return fmt.Errorf("dnspod: delete record: %w", err)
	}
	return nil
}

func (d *DNSPod) VerifyZone(ctx context.Context, zoneID string) (string, error) {
	req := dnspod.NewDescribeDomainListRequest()
	zID, err := strconv.ParseUint(zoneID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("dnspod: invalid zoneID %q: %w", zoneID, err)
	}

	resp, err := d.client.DescribeDomainListWithContext(ctx, req)
	if err != nil {
		return "", fmt.Errorf("dnspod: verify zone: %w", err)
	}
	if resp.Response != nil {
		for _, dm := range resp.Response.DomainList {
			if dm.DomainId != nil && *dm.DomainId == zID {
				return *dm.Name, nil
			}
		}
	}
	return "", fmt.Errorf("dnspod: zone ID %q not found", zoneID)
}

func getPtrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// dnspodAPIBase is the default base URL for the legacy DNSPod (dnsapi.cn) API.
// It is a package var so tests can point the provider at an httptest server.
var dnspodAPIBase = "https://dnsapi.cn"

func init() {
	Register("dnspod", func(creds []byte) (Provider, error) {
		var c dpCreds
		if err := json.Unmarshal(creds, &c); err != nil {
			return nil, fmt.Errorf("parse dnspod creds: %w", err)
		}
		// Accept either the legacy combined login token ("ID,TOKEN") or the
		// split secretId/secretKey fields, which we join into the same form.
		loginToken := strings.TrimSpace(c.Token)
		if loginToken == "" && c.SecretID != "" && c.SecretKey != "" {
			loginToken = c.SecretID + "," + c.SecretKey
		}
		if loginToken == "" {
			return nil, fmt.Errorf("dnspod: token (\"id,token\") or secretId/secretKey required")
		}
		return &DNSPod{loginToken: loginToken, base: dnspodAPIBase, hc: &http.Client{Timeout: 15 * time.Second}}, nil
	})
}

type dpCreds struct {
	Token     string `json:"token"`    // legacy: "ID,TOKEN"
	SecretID  string `json:"secretId"` // alternative split form
	SecretKey string `json:"secretKey"`
}

// DNSPod implements Provider against the legacy DNSPod login-token API
// (https://dnsapi.cn). Zones are identified by their DNSPod numeric domain_id,
// which callers store as the domain's ZoneID.
type DNSPod struct {
	loginToken string
	base       string
	hc         *http.Client
}

// dpStatus is the common status envelope returned by every dnsapi.cn endpoint.
type dpStatus struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// post performs a form POST to a dnsapi.cn endpoint, decoding into v. The
// login_token and format params are added automatically.
func (d *DNSPod) post(ctx context.Context, path string, form url.Values, v any) error {
	if form == nil {
		form = url.Values{}
	}
	form.Set("login_token", d.loginToken)
	form.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := d.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("dnspod: decode response: %w", err)
	}
	return nil
}

// dpRecord is a DNSPod record as returned by Record.List.
type dpRecord struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	TTL    string `json:"ttl"`
	MX     string `json:"mx"`
	Remark string `json:"remark"`
}

func dpToRecord(r dpRecord) Record {
	out := Record{
		ID:      r.ID,
		Type:    r.Type,
		Name:    r.Name,
		Content: r.Value,
		Comment: r.Remark,
	}
	if ttl, err := strconv.Atoi(r.TTL); err == nil {
		out.TTL = ttl
	}
	if mx, err := strconv.Atoi(r.MX); err == nil && mx > 0 {
		out.Priority = &mx
	}
	return out
}

func (d *DNSPod) ListZones(ctx context.Context) ([]Zone, error) {
	var out struct {
		Status  dpStatus `json:"status"`
		Domains []struct {
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
		} `json:"domains"`
	}
	if err := d.post(ctx, "/Domain.List", url.Values{}, &out); err != nil {
		return nil, err
	}
	if out.Status.Code != "1" {
		return nil, fmt.Errorf("dnspod: %s", out.Status.Message)
	}
	zones := make([]Zone, len(out.Domains))
	for i, z := range out.Domains {
		zones[i] = Zone{ID: z.ID.String(), Name: z.Name}
	}
	return zones, nil
}

func (d *DNSPod) ListRecords(ctx context.Context, zoneID string) ([]Record, error) {
	var out struct {
		Status  dpStatus   `json:"status"`
		Records []dpRecord `json:"records"`
	}
	form := url.Values{}
	form.Set("domain_id", zoneID)
	if err := d.post(ctx, "/Record.List", form, &out); err != nil {
		return nil, err
	}
	// Code "10" means "no records", which is not an error.
	if out.Status.Code != "1" && out.Status.Code != "10" {
		return nil, fmt.Errorf("dnspod: %s", out.Status.Message)
	}
	recs := make([]Record, len(out.Records))
	for i, r := range out.Records {
		recs[i] = dpToRecord(r)
	}
	return recs, nil
}

// recordForm builds the shared form fields for create/modify.
func recordForm(zoneID string, r Record) url.Values {
	form := url.Values{}
	form.Set("domain_id", zoneID)
	form.Set("sub_domain", subDomain(r.Name))
	form.Set("record_type", r.Type)
	form.Set("record_line", "默认") // "default" line
	form.Set("value", r.Content)
	if r.TTL > 0 {
		form.Set("ttl", strconv.Itoa(r.TTL))
	}
	if r.Priority != nil {
		form.Set("mx", strconv.Itoa(*r.Priority))
	}
	if r.Comment != "" {
		form.Set("remark", r.Comment)
	}
	return form
}

// subDomain reduces a full record name to its host portion. DNSPod expects the
// sub-domain (e.g. "www", or "@" for the apex). We pass the name through and let
// callers send the host part; an empty name becomes "@".
func subDomain(name string) string {
	if name == "" {
		return "@"
	}
	return name
}

func (d *DNSPod) CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error) {
	var out struct {
		Status dpStatus `json:"status"`
		Record struct {
			ID     json.Number `json:"id"`
			Name   string      `json:"name"`
			Value  string      `json:"value"`
			Status string      `json:"status"`
		} `json:"record"`
	}
	if err := d.post(ctx, "/Record.Create", recordForm(zoneID, r), &out); err != nil {
		return Record{}, err
	}
	if out.Status.Code != "1" {
		return Record{}, fmt.Errorf("dnspod: %s", out.Status.Message)
	}
	r.ID = out.Record.ID.String()
	return r, nil
}

func (d *DNSPod) UpdateRecord(ctx context.Context, zoneID string, r Record) (Record, error) {
	form := recordForm(zoneID, r)
	form.Set("record_id", r.ID)
	var out struct {
		Status dpStatus `json:"status"`
		Record struct {
			ID    json.Number `json:"id"`
			Name  string      `json:"name"`
			Value string      `json:"value"`
		} `json:"record"`
	}
	if err := d.post(ctx, "/Record.Modify", form, &out); err != nil {
		return Record{}, err
	}
	if out.Status.Code != "1" {
		return Record{}, fmt.Errorf("dnspod: %s", out.Status.Message)
	}
	return r, nil
}

func (d *DNSPod) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	form := url.Values{}
	form.Set("domain_id", zoneID)
	form.Set("record_id", recordID)
	var out struct {
		Status dpStatus `json:"status"`
	}
	if err := d.post(ctx, "/Record.Remove", form, &out); err != nil {
		return err
	}
	if out.Status.Code != "1" {
		return fmt.Errorf("dnspod: %s", out.Status.Message)
	}
	return nil
}

func (d *DNSPod) VerifyZone(ctx context.Context, zoneID string) (string, error) {
	form := url.Values{}
	form.Set("domain_id", zoneID)
	var out struct {
		Status dpStatus `json:"status"`
		Domain struct {
			Name string `json:"name"`
		} `json:"domain"`
	}
	if err := d.post(ctx, "/Domain.Info", form, &out); err != nil {
		return "", err
	}
	if out.Status.Code != "1" {
		return "", fmt.Errorf("dnspod: %s", out.Status.Message)
	}
	return out.Domain.Name, nil
}

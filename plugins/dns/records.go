package dns

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
)

// validateRecord catches the most common reasons Cloudflare rejects a record
// before we make the API call, returning a friendly message (or "" if valid).
func validateRecord(rec dnsprovider.Record) string {
	if strings.TrimSpace(rec.Type) == "" {
		return "record type is required"
	}
	if strings.TrimSpace(rec.Content) == "" {
		return "content is required (e.g. an IP for A, a hostname for CNAME, a value for TXT)"
	}
	switch strings.ToUpper(rec.Type) {
	case "MX", "SRV", "URI":
		if rec.Priority == nil {
			return "priority is required for " + strings.ToUpper(rec.Type) + " records"
		}
	}
	return ""
}

// --- DNS records (live via provider) ---

func (p *Plugin) recordsProvider(r *http.Request, id uint) (dnsprovider.Provider, *Domain, error) {
	var dom Domain
	if p.db.Where("id = ? AND owner_id = ?", id, p.orgID(r)).First(&dom).Error != nil {
		return nil, nil, errNotFound
	}
	prov, err := p.providerFor(dom)
	return prov, &dom, err
}

type ListRecordsInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *ListRecordsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListRecordsOutput struct {
	Body []dnsprovider.Record
}

func (p *Plugin) listRecords(ctx context.Context, input *ListRecordsInput) (*ListRecordsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	prov, dom, err := p.recordsProvider(r, input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	recs, err := prov.ListRecords(r.Context(), dom.ZoneID)
	if err != nil {
		return nil, p.providerErr("list records", err)
	}
	return &ListRecordsOutput{Body: recs}, nil
}

type CreateRecordInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body dnsprovider.Record
}

func (i *CreateRecordInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateRecordOutput struct {
	Body dnsprovider.Record
}

func (p *Plugin) createRecord(ctx context.Context, input *CreateRecordInput) (*CreateRecordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	prov, dom, err := p.recordsProvider(r, input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if dom.ZoneID == "" {
		return nil, huma.Error400BadRequest("this domain has no Zone ID — sync from Cloudflare or set it in the domain settings")
	}
	if msg := validateRecord(input.Body); msg != "" {
		return nil, huma.Error400BadRequest(msg)
	}
	out, err := prov.CreateRecord(r.Context(), dom.ZoneID, input.Body)
	if err != nil {
		return nil, p.providerErr("create record", err)
	}
	return &CreateRecordOutput{Body: out}, nil
}

type UpdateRecordInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	RID  string       `path:"rid"`
	Body dnsprovider.Record
}

func (i *UpdateRecordInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateRecordOutput struct {
	Body dnsprovider.Record
}

func (p *Plugin) updateRecord(ctx context.Context, input *UpdateRecordInput) (*UpdateRecordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	prov, dom, err := p.recordsProvider(r, input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	rec := input.Body
	rec.ID = input.RID
	if msg := validateRecord(rec); msg != "" {
		return nil, huma.Error400BadRequest(msg)
	}
	out, err := prov.UpdateRecord(r.Context(), dom.ZoneID, rec)
	if err != nil {
		return nil, p.providerErr("update record", err)
	}
	return &UpdateRecordOutput{Body: out}, nil
}

type DeleteRecordInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
	RID string       `path:"rid"`
}

func (i *DeleteRecordInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteRecordOutput struct {
	Body map[string]any
}

func (p *Plugin) deleteRecord(ctx context.Context, input *DeleteRecordInput) (*DeleteRecordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	prov, dom, err := p.recordsProvider(r, input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := prov.DeleteRecord(r.Context(), dom.ZoneID, input.RID); err != nil {
		return nil, p.providerErr("delete record", err)
	}
	return &DeleteRecordOutput{Body: map[string]any{"ok": true}}, nil
}

type dnsRecordStatus struct {
	Set     bool   `json:"set"`
	Healthy bool   `json:"healthy"`
	Value   string `json:"value,omitempty"`
}

type dnsDKIMStatus struct {
	dnsRecordStatus
	Selector string `json:"selector,omitempty"`
}

// hostDNSStatus is the SPF/DMARC/DKIM posture for a single mail hostname.
type hostDNSStatus struct {
	Host  string          `json:"host"`
	SPF   dnsRecordStatus `json:"spf"`
	DMARC dnsRecordStatus `json:"dmarc"`
	DKIM  dnsDKIMStatus   `json:"dkim"`
}

// checkHostDNS resolves and evaluates the SPF, DMARC and DKIM records for one
// hostname. SPF/DMARC/DKIM are per-host records, so a domain that runs mail on
// a subdomain (e.g. mail.example.com) must be probed at that subdomain rather
// than the apex.
func (p *Plugin) checkHostDNS(host string) hostDNSStatus {
	status := hostDNSStatus{Host: host}

	// 1. SPF record — lives directly on the host as a TXT record.
	spfRecords, _ := p.lookupTXT(host)
	for _, record := range spfRecords {
		lower := strings.ToLower(strings.TrimSpace(record))
		if strings.Contains(lower, "v=spf1") {
			status.SPF.Set = true
			status.SPF.Value = record
			if strings.HasPrefix(strings.ReplaceAll(lower, " ", ""), "v=spf1") {
				status.SPF.Healthy = true
			}
			break
		}
	}

	// 2. DMARC record — published at _dmarc.<host>.
	dmarcRecords, _ := p.lookupTXT("_dmarc." + host)
	for _, record := range dmarcRecords {
		lower := strings.ToLower(strings.TrimSpace(record))
		if strings.Contains(lower, "v=dmarc1") {
			status.DMARC.Set = true
			status.DMARC.Value = record
			if strings.Contains(lower, "p=none") || strings.Contains(lower, "p=quarantine") || strings.Contains(lower, "p=reject") {
				status.DMARC.Healthy = true
			}
			break
		}
	}

	// 3. DKIM record — probe common selectors at <selector>._domainkey.<host>.
	selectors := []string{"default", "octarq", "google", "mail", "k1", "sig1"}
	type dkimResult struct {
		selector string
		value    string
		set      bool
		healthy  bool
	}
	resultChan := make(chan dkimResult, len(selectors))
	for _, sel := range selectors {
		go func(s string) {
			records, err := p.lookupTXT(s + "._domainkey." + host)
			if err == nil {
				for _, r := range records {
					lower := strings.ToLower(strings.TrimSpace(r))
					if strings.Contains(lower, "v=dkim1") || strings.Contains(lower, "k=rsa") || strings.Contains(lower, "p=") {
						resultChan <- dkimResult{
							selector: s,
							value:    r,
							set:      true,
							healthy:  strings.Contains(lower, "p="),
						}
						return
					}
				}
			}
			resultChan <- dkimResult{}
		}(sel)
	}
	for i := 0; i < len(selectors); i++ {
		if res := <-resultChan; res.set {
			status.DKIM = dnsDKIMStatus{
				dnsRecordStatus: dnsRecordStatus{Set: res.set, Healthy: res.healthy, Value: res.value},
				Selector:        res.selector,
			}
		}
	}

	return status
}

// linkHostStatus is the CNAME-resolution posture for a single short-link host.
type linkHostStatus struct {
	Host    string `json:"host"`
	Set     bool   `json:"set"`             // resolves (has a CNAME record at all)
	Healthy bool   `json:"healthy"`         // CNAME points into the domain's zone
	CNAME   string `json:"cname,omitempty"` // observed CNAME target
	Target  string `json:"target"`          // expected target (the apex domain)
}

// checkLinkHost verifies that a short-link host is a CNAME pointing at the app.
// The recommended setup CNAMEs each link host to the apex domain, so a target
// that equals or sits within the zone is healthy. A host that only has an A
// record (or a Cloudflare-proxied/flattened CNAME resolving to itself) counts
// as resolving but unverified — the setup guide covers those cases.
func (p *Plugin) checkLinkHost(host, apex string) linkHostStatus {
	st := linkHostStatus{Host: host, Target: apex}
	cname, err := p.lookupCNAME(host)
	if err != nil {
		return st // NXDOMAIN / no resolution
	}
	cname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cname)), ".")
	host = strings.ToLower(host)
	apex = strings.ToLower(apex)
	if cname == "" || cname == host {
		// No external CNAME (A-only or proxied flattening) — resolves but the
		// target can't be confirmed via DNS.
		st.Set = cname != ""
		return st
	}
	st.CNAME = cname
	st.Set = true
	if cname == apex || strings.HasSuffix(cname, "."+apex) {
		st.Healthy = true
	}
	return st
}

type VerifyDomainDNSInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *VerifyDomainDNSInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type VerifyDomainDNSOutput struct {
	Body map[string]any
}

// GET /api/domains/{id}/verify-dns
func (p *Plugin) verifyDomainDNS(ctx context.Context, input *VerifyDomainDNSInput) (*VerifyDomainDNSOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var dom Domain
	if err := p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&dom).Error; err != nil {
		return nil, huma.Error404NotFound("not found")
	}

	// Verify every enabled mail host. Records are per-host, so a domain serving
	// mail from subdomains needs each one checked; an empty list falls back to
	// the apex (matching how mailboxes resolve elsewhere).
	hosts := dom.MailHosts.Enabled()
	if len(hosts) == 0 {
		hosts = []string{dom.Name}
	}

	results := make([]hostDNSStatus, 0, len(hosts))
	for _, host := range hosts {
		results = append(results, p.checkHostDNS(host))
	}

	// Top-level fields describe the apex (or the first host when the apex isn't
	// itself a mail host), preserving the original single-host response shape.
	primary := results[0]
	for _, res := range results {
		if res.Host == dom.Name {
			primary = res
			break
		}
	}

	// Short-link hosts are verified by CNAME resolution into the zone.
	links := make([]linkHostStatus, 0)
	for _, host := range dom.LinkHosts.Enabled() {
		links = append(links, p.checkLinkHost(host, dom.Name))
	}

	return &VerifyDomainDNSOutput{
		Body: map[string]any{
			"spf":   primary.SPF,
			"dmarc": primary.DMARC,
			"dkim":  primary.DKIM,
			"hosts": results,
			"links": links,
		},
	}, nil
}

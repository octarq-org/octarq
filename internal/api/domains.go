package api

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/internal/models"
)

// providerErr logs an upstream DNS-provider failure and returns it as a 400 so
// the real message reaches the browser. (A 5xx would be replaced by the
// Cloudflare proxy's own error page, hiding the cause.)
func (h *Handler) providerErr(action string, err error) error {
	log.Printf("dns provider: %s failed: %v", action, err)
	return huma.Error400BadRequest(action + ": " + err.Error())
}

type DNSProvidersInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *DNSProvidersInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DNSProvidersOutput struct {
	Body []string
}

func (h *Handler) dnsProviders(ctx context.Context, input *DNSProvidersInput) (*DNSProvidersOutput, error) {
	return &DNSProvidersOutput{Body: dnsprovider.Names()}, nil
}

// normalizeHost cleans a user-supplied host into a bare lowercase hostname
// (no scheme, no path, no trailing dot).
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	host = strings.TrimSuffix(host, ".")
	if i := strings.IndexAny(host, "/:"); i >= 0 {
		host = host[:i]
	}
	return host
}

// hostEntry is a host with its enable flag in create/update payloads.
type hostEntry struct {
	Host    string `json:"host"`
	Enabled *bool  `json:"enabled"`
}

// normalizeHosts cleans and de-duplicates a host list, preserving each host's
// enabled flag (defaulting to enabled). A service (links/mail) is considered
// configured when its host list is non-empty — there is no separate toggle.
func normalizeHosts(hosts []hostEntry) models.HostList {
	seen := map[string]bool{}
	var out models.HostList
	for _, h := range hosts {
		name := normalizeHost(h.Host)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		enabled := true
		if h.Enabled != nil {
			enabled = *h.Enabled
		}
		out = append(out, models.Host{Host: name, Enabled: enabled})
	}
	return out
}

type SyncDomainsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		ProviderAccountID uint `json:"providerAccountId,omitempty"`
	}
}

func (i *SyncDomainsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type SyncDomainsOutput struct {
	Body map[string]any
}

// syncDomains imports every zone the given credentials can access, creating a
// Domain for each new zone and refreshing the zone id / stored credentials on
// existing ones. User flags (forLink/forMail/note) on existing domains are kept.
func (h *Handler) syncDomains(ctx context.Context, input *SyncDomainsInput) (*SyncDomainsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if input.Body.ProviderAccountID == 0 {
		return nil, huma.Error400BadRequest("providerAccountId is required")
	}
	var acc models.ProviderAccount
	if err := h.db.Where("id = ? AND owner_id = ?", input.Body.ProviderAccountID, h.orgID(r)).First(&acc).Error; err != nil {
		return nil, huma.Error404NotFound("provider account not found")
	}

	creds, err := h.cipher.Decrypt(acc.Config)
	if err != nil {
		return nil, huma.Error500InternalServerError("stored API token could not be decrypted — re-save this provider's API token under Settings → DNS Providers (the encryption key or database changed since it was saved)")
	}

	prov, err := dnsprovider.New(acc.Type, creds)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	zones, err := prov.ListZones(r.Context())
	if err != nil {
		return nil, h.providerErr("list zones", err)
	}
	var created, updated int
	for _, z := range zones {
		name := strings.ToLower(z.Name)
		var dom models.Domain
		if h.db.Where("name = ? AND owner_id = ?", name, h.orgID(r)).First(&dom).Error == nil {
			dom.ZoneID = z.ID
			dom.ProviderAccountID = acc.ID
			h.db.Save(&dom)
			updated++
		} else {
			h.db.Create(&models.Domain{
				OrgID: h.orgID(r),
				Name:  name, ProviderAccountID: acc.ID, ZoneID: z.ID,
			})
			created++
		}
	}
	return &SyncDomainsOutput{
		Body: map[string]any{
			"ok": true, "total": len(zones), "created": created, "updated": updated,
		},
	}, nil
}

// domainDTO is the create/update payload.
// LinkHosts/MailHosts are pointers so we can distinguish "not sent" (nil)
// from "explicitly set to empty" ([]) in PATCH-style updates.
type domainDTO struct {
	Name              string `json:"name,omitempty"`
	ProviderAccountID uint   `json:"providerAccountId,omitempty"`
	ZoneID            string `json:"zoneId,omitempty"`
	Note              string `json:"note,omitempty"`
	// ForLink/ForMail are pointer booleans so "not sent" (nil) is distinct
	// from an explicit true/false, enabling domain-level master toggles that
	// are independent of the individual host lists.
	ForMail   *bool        `json:"forMail,omitempty"`
	ForLink   *bool        `json:"forLink,omitempty"`
	LinkHosts *[]hostEntry `json:"linkHosts,omitempty"`
	MailHosts *[]hostEntry `json:"mailHosts,omitempty"`
}

type ListDomainsInput struct {
	Ctx    huma.Context `hidden:"true"`
	Q      string       `query:"q"`
	Limit  int          `query:"limit"`
	Offset int          `query:"offset"`
}

func (i *ListDomainsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListDomainsOutput struct {
	Body []models.Domain
}

func (h *Handler) listDomains(ctx context.Context, input *ListDomainsInput) (*ListDomainsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var ds []models.Domain
	q := h.orgDB(r).Order("created_at DESC")
	if input.Q != "" {
		like := "%" + input.Q + "%"
		q = q.Where("name LIKE ? OR note LIKE ?", like, like)
	}
	limit := 50
	if input.Limit > 0 && input.Limit <= 500 {
		limit = input.Limit
	}
	offset := 0
	if input.Offset > 0 {
		offset = input.Offset
	}
	q = q.Limit(limit).Offset(offset)
	q.Find(&ds)
	return &ListDomainsOutput{Body: ds}, nil
}

// ownsProviderAccount reports whether the given provider account id belongs to
// the caller's org. Guards against binding another tenant's DNS credentials.
func (h *Handler) ownsProviderAccount(r *http.Request, id uint) bool {
	if id == 0 {
		return false
	}
	var acc models.ProviderAccount
	return h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&acc).Error == nil
}

type CreateDomainInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body domainDTO
}

func (i *CreateDomainInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateDomainOutput struct {
	Body models.Domain
}

func (h *Handler) createDomain(ctx context.Context, input *CreateDomainInput) (*CreateDomainOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	name := strings.TrimSpace(strings.ToLower(input.Body.Name))
	if name == "" || input.Body.ProviderAccountID == 0 {
		return nil, huma.Error400BadRequest("name and provider account are required")
	}

	if !h.ownsProviderAccount(r, input.Body.ProviderAccountID) {
		return nil, huma.Error404NotFound("provider account not found")
	}
	var linkHosts, mailHosts []hostEntry
	if input.Body.LinkHosts != nil {
		linkHosts = *input.Body.LinkHosts
	}
	if input.Body.MailHosts != nil {
		mailHosts = *input.Body.MailHosts
	}
	dom := models.Domain{
		OrgID:             h.orgID(r),
		Name:              name,
		ProviderAccountID: input.Body.ProviderAccountID,
		ZoneID:            input.Body.ZoneID,
		Note:              input.Body.Note,
		LinkHosts:         normalizeHosts(linkHosts),
		MailHosts:         normalizeHosts(mailHosts),
	}
	// On creation, derive master switches from host presence unless explicitly set.
	if input.Body.ForLink != nil {
		dom.ForLink = *input.Body.ForLink
	} else {
		dom.ForLink = len(dom.LinkHosts) > 0
	}
	if input.Body.ForMail != nil {
		dom.ForMail = *input.Body.ForMail
	} else {
		dom.ForMail = len(dom.MailHosts) > 0
	}
	// Best-effort credential check.
	if prov, err := h.providerFor(dom); err == nil && dom.ZoneID != "" {
		if name, err := prov.VerifyZone(r.Context(), dom.ZoneID); err != nil {
			return nil, huma.Error400BadRequest("provider verification failed: " + err.Error())
		} else if dom.Name == "" {
			dom.Name = name
		}
	}
	if err := h.db.Create(&dom).Error; err != nil {
		return nil, huma.NewError(http.StatusConflict, "domain already exists")
	}
	h.audit(r, "domain.create", "domain", dom.ID, map[string]any{"name": dom.Name})
	return &CreateDomainOutput{Body: dom}, nil
}

type UpdateDomainInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body domainDTO
}

func (i *UpdateDomainInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateDomainOutput struct {
	Body models.Domain
}

func (h *Handler) updateDomain(ctx context.Context, input *UpdateDomainInput) (*UpdateDomainOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var dom models.Domain
	if h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).First(&dom).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	dom.Note = input.Body.Note
	dom.ZoneID = input.Body.ZoneID
	// Apply master switches when explicitly provided.
	if input.Body.ForLink != nil {
		dom.ForLink = *input.Body.ForLink
	}
	if input.Body.ForMail != nil {
		dom.ForMail = *input.Body.ForMail
	}
	// Only overwrite host lists when they were present in the payload.
	if input.Body.LinkHosts != nil {
		dom.LinkHosts = normalizeHosts(*input.Body.LinkHosts)
	}
	if input.Body.MailHosts != nil {
		dom.MailHosts = normalizeHosts(*input.Body.MailHosts)
	}
	if input.Body.ProviderAccountID != 0 {
		if !h.ownsProviderAccount(r, input.Body.ProviderAccountID) {
			return nil, huma.Error404NotFound("provider account not found")
		}
		dom.ProviderAccountID = input.Body.ProviderAccountID
	}
	h.db.Save(&dom)
	h.audit(r, "domain.update", "domain", dom.ID, map[string]any{"name": dom.Name})
	return &UpdateDomainOutput{Body: dom}, nil
}

type DeleteDomainInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteDomainInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteDomainOutput struct {
	Body map[string]bool
}

func (h *Handler) deleteDomain(ctx context.Context, input *DeleteDomainInput) (*DeleteDomainOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if res := h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).Delete(&models.Domain{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	h.audit(r, "domain.delete", "domain", input.ID, nil)
	return &DeleteDomainOutput{Body: map[string]bool{"ok": true}}, nil
}

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

func (h *Handler) recordsProvider(r *http.Request, id uint) (dnsprovider.Provider, *models.Domain, error) {
	var dom models.Domain
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&dom).Error != nil {
		return nil, nil, errNotFound
	}
	prov, err := h.providerFor(dom)
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

func (h *Handler) listRecords(ctx context.Context, input *ListRecordsInput) (*ListRecordsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	prov, dom, err := h.recordsProvider(r, input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	recs, err := prov.ListRecords(r.Context(), dom.ZoneID)
	if err != nil {
		return nil, h.providerErr("list records", err)
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

func (h *Handler) createRecord(ctx context.Context, input *CreateRecordInput) (*CreateRecordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	prov, dom, err := h.recordsProvider(r, input.ID)
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
		return nil, h.providerErr("create record", err)
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

func (h *Handler) updateRecord(ctx context.Context, input *UpdateRecordInput) (*UpdateRecordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	prov, dom, err := h.recordsProvider(r, input.ID)
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
		return nil, h.providerErr("update record", err)
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

func (h *Handler) deleteRecord(ctx context.Context, input *DeleteRecordInput) (*DeleteRecordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	prov, dom, err := h.recordsProvider(r, input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := prov.DeleteRecord(r.Context(), dom.ZoneID, input.RID); err != nil {
		return nil, h.providerErr("delete record", err)
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
func (h *Handler) checkHostDNS(host string) hostDNSStatus {
	status := hostDNSStatus{Host: host}

	// 1. SPF record — lives directly on the host as a TXT record.
	spfRecords, _ := h.lookupTXT(host)
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
	dmarcRecords, _ := h.lookupTXT("_dmarc." + host)
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
			records, err := h.lookupTXT(s + "._domainkey." + host)
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
func (h *Handler) checkLinkHost(host, apex string) linkHostStatus {
	st := linkHostStatus{Host: host, Target: apex}
	cname, err := h.lookupCNAME(host)
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
func (h *Handler) verifyDomainDNS(ctx context.Context, input *VerifyDomainDNSInput) (*VerifyDomainDNSOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var dom models.Domain
	if err := h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).First(&dom).Error; err != nil {
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
		results = append(results, h.checkHostDNS(host))
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
		links = append(links, h.checkLinkHost(host, dom.Name))
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

package dns

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
func (p *Plugin) providerErr(action string, err error) error {
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

func (p *Plugin) dnsProviders(ctx context.Context, input *DNSProvidersInput) (*DNSProvidersOutput, error) {
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
func (p *Plugin) syncDomains(ctx context.Context, input *SyncDomainsInput) (*SyncDomainsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if input.Body.ProviderAccountID == 0 {
		return nil, huma.Error400BadRequest("providerAccountId is required")
	}
	var acc ProviderAccount
	if err := p.db.Where("id = ? AND owner_id = ?", input.Body.ProviderAccountID, p.orgID(r)).First(&acc).Error; err != nil {
		return nil, huma.Error404NotFound("provider account not found")
	}

	creds, err := p.decrypt(acc.Config)
	if err != nil {
		return nil, huma.Error500InternalServerError("stored API token could not be decrypted — re-save this provider's API token under Settings → DNS Providers (the encryption key or database changed since it was saved)")
	}

	prov, err := dnsprovider.New(acc.Type, creds)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	zones, err := prov.ListZones(r.Context())
	if err != nil {
		return nil, p.providerErr("list zones", err)
	}
	var created, updated int
	for _, z := range zones {
		name := strings.ToLower(z.Name)
		var dom Domain
		if p.db.Where("name = ? AND owner_id = ?", name, p.orgID(r)).First(&dom).Error == nil {
			dom.ZoneID = z.ID
			dom.ProviderAccountID = acc.ID
			p.db.Save(&dom)
			updated++
		} else {
			p.db.Create(&Domain{
				OrgID: p.orgID(r),
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
	Body []Domain
}

func (p *Plugin) listDomains(ctx context.Context, input *ListDomainsInput) (*ListDomainsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var ds []Domain
	q := p.orgDB(r).Order("created_at DESC")
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
func (p *Plugin) ownsProviderAccount(r *http.Request, id uint) bool {
	if id == 0 {
		return false
	}
	var acc ProviderAccount
	return p.db.Where("id = ? AND owner_id = ?", id, p.orgID(r)).First(&acc).Error == nil
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
	Body Domain
}

func (p *Plugin) createDomain(ctx context.Context, input *CreateDomainInput) (*CreateDomainOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	name := strings.TrimSpace(strings.ToLower(input.Body.Name))
	if name == "" || input.Body.ProviderAccountID == 0 {
		return nil, huma.Error400BadRequest("name and provider account are required")
	}

	if !p.ownsProviderAccount(r, input.Body.ProviderAccountID) {
		return nil, huma.Error404NotFound("provider account not found")
	}
	var linkHosts, mailHosts []hostEntry
	if input.Body.LinkHosts != nil {
		linkHosts = *input.Body.LinkHosts
	}
	if input.Body.MailHosts != nil {
		mailHosts = *input.Body.MailHosts
	}
	dom := Domain{
		OrgID:             p.orgID(r),
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
	if prov, err := p.providerFor(dom); err == nil && dom.ZoneID != "" {
		if name, err := prov.VerifyZone(r.Context(), dom.ZoneID); err != nil {
			if p.publishEvent != nil {
				p.publishEvent(dom.OrgID, "domain.verify_failed", map[string]any{"name": dom.Name, "zoneId": dom.ZoneID, "error": err.Error()})
			}
			return nil, huma.Error400BadRequest("provider verification failed: " + err.Error())
		} else if dom.Name == "" {
			dom.Name = name
		}
	}
	if err := p.db.Create(&dom).Error; err != nil {
		return nil, huma.NewError(http.StatusConflict, "domain already exists")
	}
	p.audit(r, "domain.create", "domain", dom.ID, map[string]any{"name": dom.Name})
	if p.publishEvent != nil {
		p.publishEvent(dom.OrgID, "domain.create", map[string]any{"id": dom.ID, "name": dom.Name})
	}
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
	Body Domain
}

func (p *Plugin) updateDomain(ctx context.Context, input *UpdateDomainInput) (*UpdateDomainOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var dom Domain
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&dom).Error != nil {
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
		if !p.ownsProviderAccount(r, input.Body.ProviderAccountID) {
			return nil, huma.Error404NotFound("provider account not found")
		}
		dom.ProviderAccountID = input.Body.ProviderAccountID
	}
	p.db.Save(&dom)
	p.audit(r, "domain.update", "domain", dom.ID, map[string]any{"name": dom.Name})
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

func (p *Plugin) deleteDomain(ctx context.Context, input *DeleteDomainInput) (*DeleteDomainOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if res := p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).Delete(&Domain{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	p.audit(r, "domain.delete", "domain", input.ID, nil)
	return &DeleteDomainOutput{Body: map[string]bool{"ok": true}}, nil
}

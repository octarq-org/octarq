package dns

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

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
	limit := plugin.PageLimit(input.Limit, 50, 500)
	offset := plugin.PageOffset(input.Offset)
	q = q.Limit(limit).Offset(offset)
	q.Find(&ds)
	return &ListDomainsOutput{Body: ds}, nil
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
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to create domain")
	}
	name := strings.TrimSpace(strings.ToLower(input.Body.Name))
	if name == "" || input.Body.ProviderAccountID == 0 {
		return nil, huma.Error400BadRequest("name and provider account are required")
	}
	if underBaseZone(p.db, name) {
		return nil, huma.Error400BadRequest("that hostname is reserved for automatic tenant subdomains")
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
	// The reserved zone applies to the host lists as well as the name: a link
	// or mail host under the base would claim a label no org owns yet.
	if bad := reservedInHostLists(p.db, dom.LinkHosts, dom.MailHosts); bad != "" {
		return nil, huma.Error400BadRequest("that hostname is reserved for automatic tenant subdomains")
	}
	// Host lists must stay inside this domain's own zone — the apex itself or
	// a subdomain of it. Domain.Name is unique, so this is what keeps two orgs
	// from claiming the same hostname through their lists.
	if bad := hostsOutsideZone(dom.Name, dom.LinkHosts, dom.MailHosts); bad != "" {
		return nil, huma.Error400BadRequest("host list entry " + bad + " is outside this domain's zone")
	}
	// Best-effort credential check.
	if prov, err := p.providerFor(dom); err == nil && dom.ZoneID != "" {
		if name, err := prov.VerifyZone(r.Context(), dom.ZoneID); err != nil {
			if p.publishEvent != nil {
				p.publishEvent(dom.OrgID, "domain.verify_failed", map[string]any{"name": dom.Name, "zoneId": dom.ZoneID, "error": err.Error()})
			}
			return nil, p.providerErr("verify zone", err)
		} else if dom.Name == "" {
			dom.Name = name
		}
	}
	// Metered consumption: a custom domain is a metered resource on the hosted
	// build, so ask the (hosted-only) quota checker whether this org may add
	// one. Self-hosted has no checker and this always passes there. The check
	// runs before the row exists so a refusal leaves no partial domain behind.
	if err := plugin.CheckQuota(p.ctx, ctx, dom.OrgID, "customDomains", 1); err != nil {
		if errors.Is(err, plugin.ErrQuotaUnavailable) {
			return nil, huma.Error402PaymentRequired("custom domains are not included in this plan")
		}
		return nil, huma.Error429TooManyRequests("custom domain quota exceeded for this workspace")
	}
	if err := p.db.Create(&dom).Error; err != nil {
		return nil, huma.NewError(http.StatusConflict, "domain already exists")
	}
	forgetOrigin(dom.Name)
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
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to update domain")
	}
	var dom Domain
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&dom).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	if input.Body.Note != "" {
		dom.Note = input.Body.Note
	}
	if input.Body.ZoneID != "" {
		dom.ZoneID = input.Body.ZoneID
	}
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
	// The reserved zone applies to the host lists as well as the name.
	if bad := reservedInHostLists(p.db, dom.LinkHosts, dom.MailHosts); bad != "" {
		return nil, huma.Error400BadRequest("that hostname is reserved for automatic tenant subdomains")
	}
	// The lists must stay inside this domain's own zone, exactly as on create.
	if bad := hostsOutsideZone(dom.Name, dom.LinkHosts, dom.MailHosts); bad != "" {
		return nil, huma.Error400BadRequest("host list entry " + bad + " is outside this domain's zone")
	}
	if input.Body.ProviderAccountID != 0 {
		if !p.ownsProviderAccount(r, input.Body.ProviderAccountID) {
			return nil, huma.Error404NotFound("provider account not found")
		}
		dom.ProviderAccountID = input.Body.ProviderAccountID
	}
	p.db.Save(&dom)
	// The link/mail host lists moved even when the name did not, and those are
	// what ServesTraffic answers from.
	forgetOrigin(dom.Name)
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
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to delete domain")
	}
	// Read the name before the row goes: it is the cache key that has to be
	// dropped, and after the DELETE there is nowhere left to read it from.
	var dom Domain
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&dom).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	p.db.Where("domain_id = ? AND owner_id = ?", input.ID, p.orgID(r)).Delete(&DDNSToken{})
	if res := p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).Delete(&Domain{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	forgetOrigin(dom.Name)
	p.audit(r, "domain.delete", "domain", input.ID, nil)
	return &DeleteDomainOutput{Body: map[string]bool{"ok": true}}, nil
}

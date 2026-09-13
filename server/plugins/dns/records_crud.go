package dns

import (
	"context"
	"errors"
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

// recordsProvider loads the domain (scoped to the caller's org) and builds its
// DNS provider.
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
		if errors.Is(err, errNotFound) {
			return nil, huma.Error404NotFound("domain not found")
		}
		return nil, huma.Error400BadRequest(err.Error())
	}
	if dom.ZoneID == "" {
		return nil, huma.Error400BadRequest("this domain has no Zone ID — sync from Cloudflare or set it in the domain settings")
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
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to create DNS record")
	}
	prov, dom, err := p.recordsProvider(r, input.ID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, huma.Error404NotFound("domain not found")
		}
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
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to update DNS record")
	}
	prov, dom, err := p.recordsProvider(r, input.ID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, huma.Error404NotFound("domain not found")
		}
		return nil, huma.Error400BadRequest(err.Error())
	}
	if dom.ZoneID == "" {
		return nil, huma.Error400BadRequest("this domain has no Zone ID — sync from Cloudflare or set it in the domain settings")
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
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to delete DNS record")
	}
	prov, dom, err := p.recordsProvider(r, input.ID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, huma.Error404NotFound("domain not found")
		}
		return nil, huma.Error400BadRequest(err.Error())
	}
	if dom.ZoneID == "" {
		return nil, huma.Error400BadRequest("this domain has no Zone ID — sync from Cloudflare or set it in the domain settings")
	}
	if err := prov.DeleteRecord(r.Context(), dom.ZoneID, input.RID); err != nil {
		return nil, p.providerErr("delete record", err)
	}
	return &DeleteRecordOutput{Body: map[string]any{"ok": true}}, nil
}

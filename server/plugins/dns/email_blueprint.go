package dns

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
)

// BlueprintStatus represents how a recommended email DNS record compares to the
// live provider state.
type BlueprintStatus string

const (
	// BlueprintStatusOK means the record exists and its content matches.
	BlueprintStatusOK BlueprintStatus = "ok"
	// BlueprintStatusMissing means the record does not exist in the provider.
	BlueprintStatusMissing BlueprintStatus = "missing"
	// BlueprintStatusMismatch means a record of the same type+name exists but
	// its content differs from the recommended value.
	BlueprintStatusMismatch BlueprintStatus = "mismatch"
)

// EmailBlueprintRecord is one recommended email DNS record together with its
// current status as observed in the DNS provider.
type EmailBlueprintRecord struct {
	Type     string          `json:"type"`
	Name     string          `json:"name"`    // relative, e.g. "@" or "_dmarc"
	Content  string          `json:"content"` // recommended value
	TTL      int             `json:"ttl"`
	Priority *int            `json:"priority,omitempty"`
	Status   BlueprintStatus `json:"status"`
}

// mxPriority returns a pointer to the given integer (helper for inline literals).
func mxPriority(n int) *int { return &n }

// emailBlueprintRecords returns the hardcoded Cloudflare Email Routing template.
func emailBlueprintRecords() []EmailBlueprintRecord {
	return []EmailBlueprintRecord{
		{Type: "MX", Name: "@", Content: "route1.mx.cloudflare.net", TTL: 1, Priority: mxPriority(10)},
		{Type: "MX", Name: "@", Content: "route2.mx.cloudflare.net", TTL: 1, Priority: mxPriority(53)},
		{Type: "TXT", Name: "@", Content: "v=spf1 include:_spf.mx.cloudflare.net ~all", TTL: 1},
		{Type: "TXT", Name: "_dmarc", Content: "v=DMARC1; p=none; sp=none;", TTL: 1},
	}
}

// normaliseRecordName converts a provider-returned FQDN record name to the
// relative "@" or subdomain form used in the blueprint.
//
// Providers (Cloudflare) return names as FQDNs (e.g. "example.com",
// "_dmarc.example.com"). We strip the apex domain suffix so that blueprint
// names like "@" and "_dmarc" compare cleanly.
func normaliseRecordName(name, apex string) string {
	name = strings.TrimSuffix(name, ".")
	apex = strings.TrimSuffix(apex, ".")

	if strings.EqualFold(name, apex) {
		return "@"
	}
	suffix := "." + apex
	if strings.HasSuffix(strings.ToLower(name), strings.ToLower(suffix)) {
		return name[:len(name)-len(suffix)]
	}
	return name
}

// computeBlueprintStatus compares each blueprint record against the live
// provider records and fills in the Status field.
func computeBlueprintStatus(recs []EmailBlueprintRecord, live []dnsprovider.Record, apex string) []EmailBlueprintRecord {
	out := make([]EmailBlueprintRecord, len(recs))
	for i, bp := range recs {
		status := BlueprintStatusMissing
		for _, lr := range live {
			normName := normaliseRecordName(lr.Name, apex)
			if !strings.EqualFold(lr.Type, bp.Type) || !strings.EqualFold(normName, bp.Name) {
				continue
			}
			// Type+name match found.
			if strings.TrimSpace(lr.Content) == strings.TrimSpace(bp.Content) {
				status = BlueprintStatusOK
				break
			}
			// For MX we require type+name+content all match (multiple MX records
			// are normal). Keep looking for a content match before marking mismatch.
			if status == BlueprintStatusMissing {
				status = BlueprintStatusMismatch
			}
		}
		bp.Status = status
		out[i] = bp
	}
	return out
}

// --- GET /api/domains/{id}/email-blueprint ---

// EmailBlueprintInput is the request input for the email-blueprint endpoint.
type EmailBlueprintInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

// Resolve stores the huma context.
func (i *EmailBlueprintInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// EmailBlueprintOutput wraps the list of blueprint records.
type EmailBlueprintOutput struct {
	Body []EmailBlueprintRecord
}

func (p *Plugin) emailBlueprint(ctx context.Context, input *EmailBlueprintInput) (*EmailBlueprintOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	var dom Domain
	if err := p.orgDB(r).Where("id = ?", input.ID).First(&dom).Error; err != nil {
		return nil, huma.Error404NotFound("domain not found")
	}

	blueprint := emailBlueprintRecords()

	if dom.ProviderAccountID == 0 {
		// No provider configured — every record is unknown/missing.
		for i := range blueprint {
			blueprint[i].Status = BlueprintStatusMissing
		}
		return &EmailBlueprintOutput{Body: blueprint}, nil
	}

	prov, err := p.providerFor(dom)
	if err != nil {
		return nil, huma.Error400BadRequest("domain has no DNS provider configured")
	}

	live, err := prov.ListRecords(r.Context(), dom.ZoneID)
	if err != nil {
		return nil, p.providerErr("list records", err)
	}

	blueprint = computeBlueprintStatus(blueprint, live, dom.Name)
	return &EmailBlueprintOutput{Body: blueprint}, nil
}

// --- POST /api/domains/{id}/apply-email-blueprint ---

// ApplyEmailBlueprintInput is the request input for the apply endpoint.
type ApplyEmailBlueprintInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

// Resolve stores the huma context.
func (i *ApplyEmailBlueprintInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// ApplyEmailBlueprintOutput reports how many records were created vs skipped.
type ApplyEmailBlueprintOutput struct {
	Body struct {
		OK      bool `json:"ok"`
		Applied int  `json:"applied"`
		Skipped int  `json:"skipped"`
	}
}

func (p *Plugin) applyEmailBlueprint(ctx context.Context, input *ApplyEmailBlueprintInput) (*ApplyEmailBlueprintOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("admin role required to apply email blueprint")
	}

	var dom Domain
	if err := p.orgDB(r).Where("id = ?", input.ID).First(&dom).Error; err != nil {
		return nil, huma.Error404NotFound("domain not found")
	}

	if dom.ProviderAccountID == 0 {
		return nil, huma.Error400BadRequest("domain has no DNS provider configured")
	}

	prov, err := p.providerFor(dom)
	if err != nil {
		return nil, huma.Error400BadRequest("domain has no DNS provider configured")
	}

	live, err := prov.ListRecords(r.Context(), dom.ZoneID)
	if err != nil {
		return nil, p.providerErr("list records", err)
	}

	blueprint := computeBlueprintStatus(emailBlueprintRecords(), live, dom.Name)

	var applied, skipped int
	for _, bp := range blueprint {
		if bp.Status == BlueprintStatusOK {
			skipped++
			continue
		}
		rec := dnsprovider.Record{
			Type:     bp.Type,
			Name:     bp.Name,
			Content:  bp.Content,
			TTL:      bp.TTL,
			Priority: bp.Priority,
		}
		if _, err := prov.CreateRecord(r.Context(), dom.ZoneID, rec); err != nil {
			return nil, p.providerErr("create record", err)
		}
		applied++
	}

	orgID := p.orgID(r)
	if p.audit != nil {
		p.audit(r, "email_blueprint_applied", "domain", dom.ID, map[string]any{
			"applied": applied,
			"skipped": skipped,
		})
	}
	if p.publishEvent != nil {
		p.publishEvent(orgID, "domain.email_blueprint_applied", map[string]any{
			"domainId": dom.ID,
			"applied":  applied,
			"skipped":  skipped,
		})
	}

	out := &ApplyEmailBlueprintOutput{}
	out.Body.OK = true
	out.Body.Applied = applied
	out.Body.Skipped = skipped
	return out, nil
}

package dns

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm/clause"
)

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

func (p *Plugin) syncDomains(ctx context.Context, input *SyncDomainsInput) (*SyncDomainsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to sync domains")
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
	base := models.BaseDomain(p.db)

	var validZones []dnsprovider.Zone
	var names []string
	for _, z := range zones {
		name := strings.ToLower(z.Name)
		if base != "" {
			normalized := normalizeHost(name)
			if normalized == base || strings.HasSuffix(normalized, "."+base) {
				continue
			}
		}
		validZones = append(validZones, z)
		names = append(names, name)
	}

	existingMap := make(map[string]*Domain)
	if len(names) > 0 {
		var existingDomains []Domain
		if err := p.db.Where("owner_id = ? AND name IN ?", p.orgID(r), names).Find(&existingDomains).Error; err == nil {
			for i := range existingDomains {
				existingMap[existingDomains[i].Name] = &existingDomains[i]
			}
		}
	}

	var created, updated int
	var toUpdate []Domain
	var toCreate []Domain

	for _, z := range validZones {
		name := strings.ToLower(z.Name)
		if dom, exists := existingMap[name]; exists {
			dom.ZoneID = z.ID
			dom.ProviderAccountID = acc.ID
			toUpdate = append(toUpdate, *dom)
			updated++
		} else {
			toCreate = append(toCreate, Domain{
				OrgID:             p.orgID(r),
				Name:              name,
				ProviderAccountID: acc.ID,
				ZoneID:            z.ID,
			})
		}
	}

	if len(toUpdate) > 0 {
		p.db.Save(&toUpdate)
		for _, d := range toUpdate {
			forgetOrigin(d.Name)
		}
	}
	if len(toCreate) > 0 {
		res := p.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&toCreate, 100)
		created = int(res.RowsAffected)
		if created == len(toCreate) {
			for _, d := range toCreate {
				forgetOrigin(d.Name)
			}
		} else if created > 0 {
			toCreateNames := make([]string, len(toCreate))
			for i, d := range toCreate {
				toCreateNames[i] = d.Name
			}
			var insertedNames []string
			p.db.Model(&Domain{}).Where("owner_id = ? AND name IN ?", p.orgID(r), toCreateNames).Pluck("name", &insertedNames)
			for _, n := range insertedNames {
				forgetOrigin(n)
			}
		}
	}

	return &SyncDomainsOutput{
		Body: map[string]any{
			"ok": true, "total": len(zones), "created": created, "updated": updated,
		},
	}, nil
}

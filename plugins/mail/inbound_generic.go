package mail

import (
	"context"
	"crypto/subtle"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

type InboundGenericInput struct {
	Ctx     huma.Context `hidden:"true"`
	OrgSlug string       `path:"orgSlug"`
	Token   string       `path:"token"`
}

func (i *InboundGenericInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (p *Plugin) inboundGeneric(ctx context.Context, input *InboundGenericInput) (*InboundOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)

	var org models.Org
	if p.db.Where("slug = ?", input.OrgSlug).First(&org).Error != nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	if org.InboundToken == "" || subtle.ConstantTimeCompare([]byte(input.Token), []byte(org.InboundToken)) != 1 {
		p.recordInboundAuthFailure(r, org.ID, "generic")
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	raw, err := extractRawEmail(r)
	if err != nil || len(raw) == 0 {
		return nil, huma.Error400BadRequest("read body")
	}

	overrideTo := r.Header.Get("X-Octarq-To")
	if overrideTo == "" {
		overrideTo = r.Header.Get("X-Inbound-To")
	}

	return p.processInboundMail(ctx, org.ID, overrideTo, raw)
}

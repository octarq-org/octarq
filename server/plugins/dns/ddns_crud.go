package dns

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

type listDDNSTokensInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *listDDNSTokensInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type listDDNSTokensOutput struct {
	Body []DDNSToken
}

func (p *Plugin) listDDNSTokens(ctx context.Context, input *listDDNSTokensInput) (*listDDNSTokensOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable. Try again.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)

	var tokens []DDNSToken
	p.db.Where("owner_id = ?", orgID).Order("id desc").Find(&tokens)

	out := &listDDNSTokensOutput{Body: tokens}
	return out, nil
}

type createDDNSTokenInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		DomainID   uint   `json:"domainId"`
		RecordName string `json:"recordName"`
		RecordType string `json:"recordType"`
		Label      string `json:"label"`
	}
}

func (i *createDDNSTokenInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type createDDNSTokenOutput struct {
	Body struct {
		ID         uint      `json:"id"`
		DomainID   uint      `json:"domainId"`
		RecordName string    `json:"recordName"`
		RecordType string    `json:"recordType"`
		Label      string    `json:"label"`
		Secret     string    `json:"secret"`
		UpdateURL  string    `json:"updateUrl"`
		CreatedAt  time.Time `json:"createdAt"`
	}
}

func (p *Plugin) createDDNSToken(ctx context.Context, input *createDDNSTokenInput) (*createDDNSTokenOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable. Try again.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("Admin role required to create DDNS token. Contact a workspace admin for access.")
	}

	if input.Body.DomainID == 0 {
		return nil, huma.Error400BadRequest("Domain ID is required")
	}

	var dom Domain
	if err := p.db.Where("id = ? AND owner_id = ?", input.Body.DomainID, orgID).First(&dom).Error; err != nil {
		return nil, huma.Error404NotFound("Domain not found")
	}

	recName := strings.TrimSpace(input.Body.RecordName)
	if recName == "" {
		return nil, huma.Error400BadRequest("Record name is required")
	}

	recType := strings.ToUpper(strings.TrimSpace(input.Body.RecordType))
	if recType == "" {
		recType = "A"
	}
	if recType != "A" && recType != "AAAA" {
		return nil, huma.Error400BadRequest("Record type must be A or AAAA")
	}

	secret, err := generateDDNSSecret()
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not generate secret token. Try again.")
	}

	tokenHash := hashDDNSSecret(secret)

	tok := DDNSToken{
		OrgID:      orgID,
		DomainID:   dom.ID,
		RecordName: recName,
		RecordType: recType,
		TokenHash:  tokenHash,
		Label:      strings.TrimSpace(input.Body.Label),
		CreatedAt:  time.Now(),
	}

	if err := p.db.Create(&tok).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not save DDNS token. Try again.")
	}

	if p.audit != nil {
		p.audit(r, "ddns.token_create", "DDNSToken", tok.ID, map[string]any{"recordName": recName, "recordType": recType})
	}

	out := &createDDNSTokenOutput{}
	out.Body.ID = tok.ID
	out.Body.DomainID = tok.DomainID
	out.Body.RecordName = tok.RecordName
	out.Body.RecordType = tok.RecordType
	out.Body.Label = tok.Label
	out.Body.Secret = secret
	out.Body.UpdateURL = "/api/dns/ddns/update?token=" + secret
	out.Body.CreatedAt = tok.CreatedAt

	return out, nil
}

type deleteDDNSTokenInput struct {
	ID  uint         `path:"id"`
	Ctx huma.Context `hidden:"true"`
}

func (i *deleteDDNSTokenInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type deleteDDNSTokenOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (p *Plugin) deleteDDNSToken(ctx context.Context, input *deleteDDNSTokenInput) (*deleteDDNSTokenOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable. Try again.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)
	if orgID == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("Admin role required to delete DDNS token. Contact a workspace admin for access.")
	}

	res := p.db.Where("id = ? AND owner_id = ?", input.ID, orgID).Delete(&DDNSToken{})
	if res.Error != nil {
		return nil, huma.Error500InternalServerError("Could not delete DDNS token. Try again.")
	}
	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("DDNS token not found")
	}

	if p.audit != nil {
		p.audit(r, "ddns.token_delete", "DDNSToken", input.ID, nil)
	}

	out := &deleteDDNSTokenOutput{}
	out.Body.OK = true
	return out, nil
}

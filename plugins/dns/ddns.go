package dns

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
)

var trustProxy bool

// SetTrustProxy configures whether proxy-supplied client-IP headers are trusted for DDNS IP detection.
func SetTrustProxy(v bool) { trustProxy = v }

func clientIP(r *http.Request) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
		if rip := strings.TrimSpace(r.Header.Get("X-Real-IP")); rip != "" {
			return rip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func generateDDNSSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashDDNSSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// GET /api/dns/ddns
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
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)

	var tokens []DDNSToken
	p.db.Where("owner_id = ?", orgID).Order("id desc").Find(&tokens)

	out := &listDDNSTokensOutput{Body: tokens}
	return out, nil
}

// POST /api/dns/ddns
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
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)

	if input.Body.DomainID == 0 {
		return nil, huma.Error400BadRequest("domainId is required")
	}

	var dom Domain
	if err := p.db.Where("id = ? AND owner_id = ?", input.Body.DomainID, orgID).First(&dom).Error; err != nil {
		return nil, huma.Error404NotFound("domain not found")
	}

	recName := strings.TrimSpace(input.Body.RecordName)
	if recName == "" {
		return nil, huma.Error400BadRequest("recordName is required")
	}

	recType := strings.ToUpper(strings.TrimSpace(input.Body.RecordType))
	if recType == "" {
		recType = "A"
	}
	if recType != "A" && recType != "AAAA" {
		return nil, huma.Error400BadRequest("recordType must be A or AAAA")
	}

	secret, err := generateDDNSSecret()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate secret")
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
		return nil, huma.Error500InternalServerError("failed to save ddns token")
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

// DELETE /api/dns/ddns/{id}
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
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.orgID(r)

	res := p.db.Where("id = ? AND owner_id = ?", input.ID, orgID).Delete(&DDNSToken{})
	if res.Error != nil {
		return nil, huma.Error500InternalServerError("failed to delete ddns token")
	}
	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("ddns token not found")
	}

	if p.audit != nil {
		p.audit(r, "ddns.token_delete", "DDNSToken", input.ID, nil)
	}

	out := &deleteDDNSTokenOutput{}
	out.Body.OK = true
	return out, nil
}

// GET/POST /api/dns/ddns/update (Public)
type updateDDNSInput struct {
	Token string       `query:"token"`
	IP    string       `query:"ip"`
	Ctx   huma.Context `hidden:"true"`
}

func (i *updateDDNSInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (p *Plugin) updateDDNS(ctx context.Context, input *updateDDNSInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writeResp := func(text string) (*struct{}, error) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(text))
		return nil, nil
	}

	if r != nil {
		r.ParseForm()
	}

	token := strings.TrimSpace(input.Token)
	if token == "" && r != nil {
		token = strings.TrimSpace(r.FormValue("token"))
	}

	if token == "" {
		return writeResp("badauth")
	}

	tokenHash := hashDDNSSecret(token)

	var tok DDNSToken
	if err := p.db.Where("token_hash = ?", tokenHash).First(&tok).Error; err != nil {
		return writeResp("badauth")
	}

	ip := strings.TrimSpace(input.IP)
	if ip == "" && r != nil {
		ip = strings.TrimSpace(r.FormValue("ip"))
	}
	if ip == "" && r != nil {
		ip = clientIP(r)
	}

	if ip == "" {
		return writeResp("dnserr")
	}

	var dom Domain
	if err := p.db.Where("id = ? AND owner_id = ?", tok.DomainID, tok.OrgID).First(&dom).Error; err != nil {
		return writeResp("dnserr")
	}

	prov, err := p.providerFor(dom)
	if err != nil {
		return writeResp("dnserr")
	}

	recs, err := prov.ListRecords(ctx, dom.ZoneID)
	if err != nil {
		return writeResp("dnserr")
	}

	var existing *dnsprovider.Record
	for _, rec := range recs {
		if strings.EqualFold(rec.Name, tok.RecordName) && strings.EqualFold(rec.Type, tok.RecordType) {
			rCopy := rec
			existing = &rCopy
			break
		}
	}

	now := time.Now()

	if existing != nil {
		if existing.Content == ip {
			p.db.Model(&tok).Updates(map[string]any{
				"last_ip":      ip,
				"last_seen_at": &now,
			})
			return writeResp("nochg " + ip)
		}

		updated := *existing
		updated.Content = ip
		if _, err := prov.UpdateRecord(ctx, dom.ZoneID, updated); err != nil {
			return writeResp("dnserr")
		}

		p.db.Model(&tok).Updates(map[string]any{
			"last_ip":      ip,
			"last_seen_at": &now,
		})
		return writeResp("good " + ip)
	}

	// Not existing -> Create record with TTL 60
	newRec := dnsprovider.Record{
		Type:    tok.RecordType,
		Name:    tok.RecordName,
		Content: ip,
		TTL:     60,
	}
	if _, err := prov.CreateRecord(ctx, dom.ZoneID, newRec); err != nil {
		return writeResp("dnserr")
	}

	p.db.Model(&tok).Updates(map[string]any{
		"last_ip":      ip,
		"last_seen_at": &now,
	})
	return writeResp("good " + ip)
}

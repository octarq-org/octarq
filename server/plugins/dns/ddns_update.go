package dns

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
)

type updateDDNSInput struct {
	Token string       `query:"token"`
	IP    string       `query:"ip"`
	Ctx   huma.Context `hidden:"true"`
}

func (i *updateDDNSInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func recordDDNSUpdate(p *Plugin, tok *DDNSToken, ip string, now time.Time) {
	p.db.Model(tok).Updates(map[string]any{
		"last_ip":      ip,
		"last_seen_at": &now,
	})
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
			recordDDNSUpdate(p, &tok, ip, now)
			return writeResp("nochg " + ip)
		}

		updated := *existing
		updated.Content = ip
		if _, err := prov.UpdateRecord(ctx, dom.ZoneID, updated); err != nil {
			return writeResp("dnserr")
		}

		recordDDNSUpdate(p, &tok, ip, now)
		return writeResp("good " + ip)
	}

	newRec := dnsprovider.Record{
		Type:    tok.RecordType,
		Name:    tok.RecordName,
		Content: ip,
		TTL:     60,
	}
	if _, err := prov.CreateRecord(ctx, dom.ZoneID, newRec); err != nil {
		return writeResp("dnserr")
	}

	recordDDNSUpdate(p, &tok, ip, now)
	return writeResp("good " + ip)
}

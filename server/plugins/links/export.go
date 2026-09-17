package links

import (
	"context"
	"encoding/csv"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

type ExportLinksCSVInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ExportLinksCSVInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (p *Plugin) exportLinksCSV(ctx context.Context, input *ExportLinksCSVInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var links []Link
	tdb := p.tenantDB(p.orgID(r))
	if tdb == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	tdb.Order("created_at DESC").Find(&links)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"links.csv\"")

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ID", "Host", "Slug", "Target", "Title", "Clicks", "CreatedAt"})
	for _, l := range links {
		_ = cw.Write([]string{
			fmt.Sprintf("%d", l.ID),
			l.Host,
			l.Slug,
			l.Target,
			l.Title,
			fmt.Sprintf("%d", l.Clicks),
			l.CreatedAt.Format(time.RFC3339),
		})
	}
	cw.Flush()
	return nil, nil
}

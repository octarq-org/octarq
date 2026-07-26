package help

import (
	"bytes"
	"context"
	"log"
	"sort"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
)

type Plugin struct {
	pctx *plugin.Context
}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "help" }

func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:       "Help & Resources",
		Description: "In-app documentation and support resources.",
		Core:        true,
	}
}

func (p *Plugin) Models() []any { return nil }

// Menus places Help in the sidebar footer rail rather than a nav group.
// areaForCategory (web/src/shell/areas.tsx) reserves category "footer" for
// low-frequency, always-available product links and names Help as the example;
// inventing a "Help & Resources" group instead would put it in the main nav and
// oblige every locale to translate a heading that the IA does not want.
func (p *Plugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "help", Label: "Help", Path: "/help", Icon: "book", Category: "footer"},
	}
}

type DocMeta struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Group string `json:"group"`
	Order int    `json:"order"`
}

type ListDocsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListDocsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListDocsOutput struct {
	Body []DocMeta
}

func (p *Plugin) listDocs(ctx context.Context, input *ListDocsInput) (*ListDocsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.pctx.OrgID(r)

	allDocs := p.getDocs(orgID)
	out := make([]DocMeta, 0, len(allDocs))
	for _, d := range allDocs {
		out = append(out, DocMeta{
			Slug:  d.Slug,
			Title: d.Title,
			Group: d.Group,
			Order: d.Order,
		})
	}
	return &ListDocsOutput{Body: out}, nil
}

type GetDocInput struct {
	Ctx  huma.Context `hidden:"true"`
	Slug string       `path:"slug"`
}

func (i *GetDocInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetDocOutput struct {
	Body struct {
		Title string `json:"title"`
		HTML  string `json:"html"`
	}
}

func (p *Plugin) getDoc(ctx context.Context, input *GetDocInput) (*GetDocOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	orgID := p.pctx.OrgID(r)

	allDocs := p.getDocs(orgID)
	var found *plugin.HelpDoc
	for _, d := range allDocs {
		if d.Slug == input.Slug {
			found = &d
			break
		}
	}
	if found == nil {
		return nil, huma.Error404NotFound("doc not found")
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(found.Markdown), &buf); err != nil {
		return nil, huma.Error500InternalServerError("failed to render markdown")
	}

	out := &GetDocOutput{}
	out.Body.Title = found.Title
	out.Body.HTML = buf.String()
	return out, nil
}

func (p *Plugin) getDocs(orgID uint) []plugin.HelpDoc {
	var docs []plugin.HelpDoc
	slugs := make(map[string]string)

	for _, pl := range p.pctx.ActivePlugins() {
		if !p.pctx.PluginActive(orgID, pl) {
			continue
		}
		if hp, ok := pl.(plugin.HelpProvider); ok {
			for _, d := range hp.HelpDocs() {
				if owner, exists := slugs[d.Slug]; exists {
					log.Printf("[help] warning: plugin %q shadowing slug %q already claimed by %q", pl.Name(), d.Slug, owner)
					d.Slug = pl.Name() + "-" + d.Slug
				}
				slugs[d.Slug] = pl.Name()
				docs = append(docs, d)
			}
		}
	}

	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Group != docs[j].Group {
			return docs[i].Group < docs[j].Group
		}
		if docs[i].Order != docs[j].Order {
			return docs[i].Order < docs[j].Order
		}
		return docs[i].Title < docs[j].Title
	})

	return docs
}

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	p.pctx = ctx
	huma.Register(ctx.Huma, huma.Operation{
		OperationID: "listHelpDocs",
		Method:      "GET",
		Path:        "/api/help/docs",
		Summary:     "List help docs",
		Tags:        []string{"Help"},
	}, p.listDocs)
	huma.Register(ctx.Huma, huma.Operation{
		OperationID: "getHelpDoc",
		Method:      "GET",
		Path:        "/api/help/docs/{slug}",
		Summary:     "Get help doc",
		Tags:        []string{"Help"},
	}, p.getDoc)
}

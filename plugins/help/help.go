package help

import (
	"bytes"
	"context"
	_ "embed"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

//go:embed getting-started-en.mdx
var gettingStartedEnDocs string

//go:embed getting-started-zh.mdx
var gettingStartedZhDocs string

var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
	_ plugin.HelpProvider = (*Plugin)(nil)
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
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Order    int    `json:"order"`
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
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = r.Header.Get("Accept-Language")
	}

	allDocs := p.getDocs(orgID, lang)
	out := make([]DocMeta, 0, len(allDocs))
	for _, d := range allDocs {
		out = append(out, DocMeta{
			Slug:     d.Slug,
			Title:    d.Title,
			Category: d.Category,
			Order:    d.Order,
		})
	}
	return &ListDocsOutput{Body: out}, nil
}

type ListCategoriesInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListCategoriesInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListCategoriesOutput struct {
	Body []plugin.HelpCategory
}

func (p *Plugin) listCategories(ctx context.Context, input *ListCategoriesInput) (*ListCategoriesOutput, error) {
	return &ListCategoriesOutput{Body: plugin.HelpCategories()}, nil
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
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = r.Header.Get("Accept-Language")
	}

	allDocs := p.getDocs(orgID, lang)
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

func (p *Plugin) getDocs(orgID uint, lang string) []plugin.HelpDoc {
	var docs []plugin.HelpDoc
	slugs := make(map[string]string)

	for _, pl := range p.pctx.ActivePlugins() {
		if !p.pctx.PluginActive(orgID, pl) {
			continue
		}
		if hp, ok := pl.(plugin.HelpProvider); ok {
			var category string
			if desc, ok := pl.(plugin.Describer); ok {
				category = desc.Describe().Category
			}
			for _, d := range hp.HelpDocs() {
				d.FillDefaults(pl.Name(), category)
				if d.Feature != "" && p.pctx.FeatureActive != nil && !p.pctx.FeatureActive(orgID, d.Feature) {
					continue
				}
				if owner, exists := slugs[d.Slug]; exists {
					log.Printf("[help] warning: plugin %q shadowing slug %q already claimed by %q", pl.Name(), d.Slug, owner)
					d.Slug = pl.Name() + "-" + d.Slug
				}
				slugs[d.Slug] = pl.Name()

				if lang != "" {
					// Deterministic lang matching: exact match first, then primary language subtag (e.g. "zh-CN" -> "zh")
					var matchedKey string
					if _, ok := d.Translations[lang]; ok {
						matchedKey = lang
					} else if idx := strings.Index(lang, "-"); idx != -1 {
						subtag := lang[:idx]
						if _, ok := d.Translations[subtag]; ok {
							matchedKey = subtag
						}
					}

					if matchedKey != "" {
						tr := d.Translations[matchedKey]
						if tr.Title != "" {
							d.Title = tr.Title
						}
						if tr.Category != "" {
							d.Category = tr.Category
						}
						if tr.Markdown != "" {
							d.Markdown = tr.Markdown
						}
					}
				}

				docs = append(docs, d)
			}
		}
	}

	sort.Slice(docs, func(i, j int) bool {
		return plugin.CompareHelpDocs(docs[i], docs[j])
	})

	return docs
}

var parsedHelpDocs = sync.OnceValue(func() []plugin.HelpDoc {
	return []plugin.HelpDoc{
		plugin.ParseHelpDocSafe(gettingStartedEnDocs).WithTranslation("zh", gettingStartedZhDocs),
	}
})

func (p *Plugin) HelpDocs() []plugin.HelpDoc {
	return parsedHelpDocs()
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
		OperationID: "listHelpCategories",
		Method:      "GET",
		Path:        "/api/help/categories",
		Summary:     "List help categories",
		Tags:        []string{"Help"},
	}, p.listCategories)
	huma.Register(ctx.Huma, huma.Operation{
		OperationID: "getHelpDoc",
		Method:      "GET",
		Path:        "/api/help/docs/{slug}",
		Summary:     "Get help doc",
		Tags:        []string{"Help"},
	}, p.getDoc)
}

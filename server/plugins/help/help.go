package help

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

// docs holds this plugin's own documentation: the platform-level pages that
// document capabilities the core owns but no plugin does — authentication,
// organizations/RBAC, API tokens, webhooks, notification channels, MCP.
//
// Those pages have no other home. "Docs live with the feature" resolves
// cleanly for links/mail/dns, which are plugins and carry their own
// docs/ directory — but auth, orgs, tokens and webhooks live in internal/ and
// app/, which nothing can hang a docs directory on. Until they are plugins, the
// help plugin is their custodian: it is already Core, already mounted in every
// build, and already the aggregator every other contributor feeds into.
//
// The directory name is the convention (plugin.HelpDocsFS) — the same one every
// other plugin follows — so this plugin loads its own pages through exactly the
// path it serves everyone else's, with no second loader to drift.
//
//go:embed docs
var docs embed.FS

var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
	_ plugin.HelpDocsFS   = (*Plugin)(nil)
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
		// Title matches the sidebar entry below.
		Title:       "Help",
		Description: "In-app documentation, versioned with the binary.",
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

	html, err := renderMarkdown(found.Markdown)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to render markdown")
	}

	out := &GetDocOutput{}
	out.Body.Title = found.Title
	out.Body.HTML = html
	return out, nil
}

// docsFSCache memoises the parse of each plugin's embedded docs directory.
// getDocs runs on every /api/help/docs request and the embedded FS cannot change
// while the process lives, so re-walking and re-parsing thirty markdown files per
// request would be pure waste. Plugins that implement HelpProvider do their own
// caching (sync.OnceValue), which is why only the FS half is cached here.
//
// The key is the plugin VALUE, not its name. Names are not unique: a distribution
// can mount both the core help plugin and a downstream help module, and both answer
// Name() == "help" on purpose, so that the two halves of the help feature share
// one feature key and one toggle. Keying this cache by name made the second half
// read back the first half's docs — every core page served twice (once under its
// own slug, once shadow-renamed to help-<slug>) and the downstream page missing
// entirely. Plugins are long-lived singletons, so the interface value is a
// stable identity.
var docsFSCache sync.Map // plugin.Plugin -> []plugin.HelpDoc

// pluginDocs returns every doc a plugin contributes, from either half of the
// contract: the docs-directory convention (plugin.HelpDocsFS) and/or a
// hand-built HelpDocs() (plugin.HelpProvider). Implementing both is legal — a
// plugin can ship static pages from disk next to pages it generates at runtime —
// so these concatenate rather than one winning.
func pluginDocs(pl plugin.Plugin) []plugin.HelpDoc {
	var docs []plugin.HelpDoc
	if fp, ok := pl.(plugin.HelpDocsFS); ok {
		if cached, hit := docsFSCache.Load(pl); hit {
			docs = append(docs, cached.([]plugin.HelpDoc)...)
		} else {
			loaded := plugin.LoadHelpDocs(fp.HelpDocsFS())
			docsFSCache.Store(pl, loaded)
			docs = append(docs, loaded...)
		}
	}
	if hp, ok := pl.(plugin.HelpProvider); ok {
		docs = append(docs, hp.HelpDocs()...)
	}
	return docs
}

func (p *Plugin) getDocs(orgID uint, lang string) []plugin.HelpDoc {
	var docs []plugin.HelpDoc
	slugs := make(map[string]string)

	for _, pl := range p.pctx.ActivePlugins() {
		if !p.pctx.PluginActive(orgID, pl) {
			continue
		}
		contributed := pluginDocs(pl)
		if len(contributed) > 0 {
			var category string
			if desc, ok := pl.(plugin.Describer); ok {
				category = desc.Describe().Category
			}
			for _, d := range contributed {
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

// HelpDocsFS hands the embedded docs/ directory to the shared walker.
func (p *Plugin) HelpDocsFS() fs.FS { return docs }

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

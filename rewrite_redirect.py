import re

with open("plugins/links/redirect.go", "r") as f:
    content = f.read()

# Replace types
content = content.replace("models.Link", "Link")
content = content.replace("models.LinkEvent", "LinkEvent")
content = content.replace("models.RoutingRule", "RoutingRule")
content = content.replace("models.RoutingRules", "RoutingRules")

# Replace internal imports and references
content = content.replace('"github.com/octarq-org/octarq/internal/cache"\n', '')
content = content.replace('"github.com/octarq-org/octarq/internal/eventbus"\n', '')
content = content.replace('"github.com/octarq-org/octarq/internal/geo"\n', '')
content = content.replace('"github.com/octarq-org/octarq/internal/models"\n', '')
content = content.replace('"gorm.io/gorm"\n', '"gorm.io/gorm"\n\t"github.com/octarq-org/octarq/plugin"\n')

# Replace Service struct
content = content.replace(
"""type Service struct {
	db    *gorm.DB
	geo   *geo.Resolver
	cache cache.Cache
}""", 
"""type Service struct {
	db  *gorm.DB
	ctx *plugin.Context
}""")

# Replace New
content = content.replace(
"""func New(db *gorm.DB, g *geo.Resolver) *Service {
	return &Service{db: db, geo: g, cache: cache.New("")}
}

func (s *Service) WithCache(c cache.Cache) *Service {
	s.cache = c
	return s
}""",
"""func New(db *gorm.DB, ctx *plugin.Context) *Service {
	return &Service{db: db, ctx: ctx}
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	path := r.URL.Path
	slug := strings.TrimPrefix(path, "/")
	if slug == "" || strings.Contains(slug, "/") {
		http.NotFound(w, r)
		return
	}

	link, ok := s.Lookup(r.Host, slug)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.Handle(w, r, link)
}""")

# Replace Lookup cache calls
content = content.replace("s.cache.Get(ctx, cacheKey, &link)", "s.ctx.CacheGet(ctx, cacheKey, &link)")
content = content.replace("s.cache.Set(ctx, cacheKey, &empty, time.Minute)", "s.ctx.CacheSet(ctx, cacheKey, &empty, time.Minute)")
content = content.replace("s.cache.Set(ctx, cacheKey, &link, 5*time.Minute)", "s.ctx.CacheSet(ctx, cacheKey, &link, 5*time.Minute)")

# Replace geo
content = content.replace("s.geo.Locate(ip)", "s.ctx.GeoLookup(ip)")
content = content.replace("geo.ParseUA(ua)", "s.ctx.ParseUA(ua)")
content = content.replace("info geo.UAInfo", "device, browser, os string")
content = content.replace("go s.record(r.Clone(context.Background()), link.OrgID, link.Slug, link.ID, ip, country, region, city, ua, info, bot)", "go s.record(r.Clone(context.Background()), link.OrgID, link.Slug, link.ID, ip, country, region, city, ua, info.Device, info.Browser, info.OS, bot)")
content = content.replace("info.OS", "os")
content = content.replace("info.Browser", "browser")
content = content.replace("info.Device", "device")
content = content.replace("func evaluateRouting(link *Link, country, device, os string) string", "func evaluateRouting(link *Link, country, device, osStr string) string")

with open("plugins/links/redirect.go", "w") as f:
    f.write(content)

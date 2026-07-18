import re

with open("plugins/links/redirect.go", "r") as f:
    content = f.read()

content = content.replace("func New(db *gorm.DB, ctx *plugin.Context) *Service", "func newRedirectEngine(db *gorm.DB, ctx *plugin.Context) *Service")
content = content.replace("var empty models.Link", "var empty Link")
content = content.replace("s.cache.Set(ctx, cacheKey, &empty, time.Minute)", "s.ctx.CacheSet(ctx, cacheKey, &empty, time.Minute)")
content = content.replace("info := s.ctx.ParseUA(ua)", "device, browser, osStr := s.ctx.ParseUA(ua)")
content = content.replace("info.Device", "device")
content = content.replace("info.OS", "osStr")
content = content.replace("info.Browser", "browser")
content = content.replace("go s.record(r.Clone(context.Background()), link.OrgID, link.Slug, link.ID, ip, country, region, city, ua, info, bot)", "go s.record(r.Clone(context.Background()), link.OrgID, link.Slug, link.ID, ip, country, region, city, ua, device, browser, osStr, bot)")
content = content.replace("info geo.UAInfo", "device, browser, osStr string")
content = content.replace("func evaluateRouting(link *Link, country, device, os string) string", "func evaluateRouting(link *Link, country, device, osStr string) string")
content = content.replace("info deviceUA", "device, browser, osStr string")

with open("plugins/links/redirect.go", "w") as f:
    f.write(content)

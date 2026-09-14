# Plugin Specification: Short Links (`plugins/links`)

## 1. Architectural Overview & Role
The `links` plugin (`plugins/links`) provides core short link creation, multi-tenant domain routing, conditional forwarding, privacy-preserving click attribution, and high-performance redirect resolution for Octarq.

- **Plugin Identifier**: `"links"` (`plugin.go:57`)
- **Plugin Metadata**:
  - Title: `"Short Links"`
  - Description: `"Short link creation, custom domain routing, and click analytics."`
  - Category: `plugin.CategoryMarketing` (`"Marketing"`)
  - Tags: `["url", "analytics", "routing"]`
  - Enabled By Default: `true`
  - Plugin Dependencies: `["dns"]` (`plugin.go:65`)
- **Implemented Interfaces**:
  - `plugin.Plugin` (`plugin.go:33`)
  - `plugin.Describer` (`plugin.go:34`)
  - `plugin.MenuProvider` (`plugin.go:35`)
  - `plugin.InstanceMenuProvider` (`plugin.go:36`)
  - `plugin.HelpDocsFS` (`plugin.go:37`)
  - `plugin.Starter` (`plugin.go:38`)
  - `plugin.LinkCreator` (`service.go:44`)
- **Binary Embedding Guarantee**: Co-located at `plugins/links/SPEC.md`. Strictly excluded from binary embedding (`//go:embed docs` in `plugin.go:76-77` only bundles user help docs under `docs/`). Zero binary bloat, zero secret leak.

---

## 2. Exact Go Types & Structs

### 2.1 Plugin Struct (`plugins/links/plugin.go:14-30`)
```go
type Plugin struct {
	db     *gorm.DB
	engine *Engine
	auth   struct {
		UserID func(r *http.Request) uint
		OrgID  func(r *http.Request) uint
	}
	audit               func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	getGlobalSetting    func(key string) string
	getWorkspaceSetting func(orgID uint, key string) string
	enqueue             func(ctx context.Context, taskType string, payload []byte) error
	deleteCache         func(ctx context.Context, key string) error
	publishEvent        func(orgID uint, event string, data any)
	requireRole         func(r *http.Request, min string) bool
	isInstanceAdmin     func(r *http.Request) bool
	ctx                 *plugin.Context
}
```

### 2.2 Compile-Time Interface Assertions (`plugins/links/plugin.go:32-51` & `service.go:44`)
```go
var (
	_ plugin.Plugin               = (*Plugin)(nil)
	_ plugin.Describer            = (*Plugin)(nil)
	_ plugin.MenuProvider         = (*Plugin)(nil)
	_ plugin.InstanceMenuProvider = (*Plugin)(nil)
	_ plugin.HelpDocsFS           = (*Plugin)(nil)
	_ plugin.Starter              = (*Plugin)(nil)
)

var (
	_ plugin.ExportFunc   = (*Plugin)(nil).exportData
	_ plugin.PurgeFunc    = (*Plugin)(nil).purge
	_ plugin.OverviewFunc = (*Plugin)(nil).overview
	_ plugin.LinkResolver = (*Plugin)(nil).resolveSlug
	_ plugin.MCPExporter  = (*Plugin)(nil).mcpExportLinks
	_ plugin.CleanupFunc  = (*Plugin)(nil).cleanupEvents
	_ plugin.LinkCreator  = (*Plugin)(nil)
)
```

### 2.3 Domain Models (`plugins/links/models.go`)
- **RoutingRule** (`models.go:10-15`):
  ```go
  type RoutingRule struct {
  	Type   string `json:"type"`             // "geo", "device", "os", "language", "split"
  	Match  string `json:"match,omitempty"`  // e.g. "US", "Mobile", "iOS", "zh-CN" (unused for split)
  	Target string `json:"target"`           // redirect URL if matched
  	Weight int    `json:"weight,omitempty"` // used by split rules: percentage [0,100]
  }
  ```
- **RoutingRules** (`models.go:16-44`):
  Slice type `RoutingRules []RoutingRule` implementing `driver.Valuer` (`Value()` lines 18-24) and `sql.Scanner` (`Scan()` lines 25-44), serializing as a JSON array.
- **Link** (`models.go:47-66`):
  ```go
  type Link struct {
  	ID           uint         `gorm:"primaryKey" json:"id"`
  	OrgID        uint         `gorm:"column:owner_id;index;default:1" json:"-"`
  	Host         string       `gorm:"size:255;index:idx_host_slug,unique" json:"host"`
  	Slug         string       `gorm:"size:255;index:idx_host_slug,unique" json:"slug"`
  	Target       string       `gorm:"type:text" json:"target"`
  	Password     string       `gorm:"size:255" json:"password"`
  	Note         string       `gorm:"type:text" json:"note"`
  	Title        string       `gorm:"size:255" json:"title"`
  	Tags         string       `gorm:"size:512" json:"tags"`
  	ExpiresAt    *time.Time   `json:"expiresAt"`
  	ExpiredURL   string       `gorm:"type:text" json:"expiredUrl"`
  	ClickLimit   int64        `gorm:"default:0" json:"clickLimit"`
  	Archived     bool         `gorm:"default:false;index" json:"archived"`
  	Enabled      bool         `gorm:"default:true" json:"enabled"`
  	RoutingRules RoutingRules `gorm:"type:text" json:"routingRules"`
  	Clicks       int64        `gorm:"default:0" json:"clicks"`
  	CreatedAt    time.Time    `json:"createdAt"`
  	UpdatedAt    time.Time    `json:"updatedAt"`
  }
  ```
- **LinkEvent** (`models.go:69-93`):
  ```go
  type LinkEvent struct {
  	ID          uint      `gorm:"primaryKey" json:"id"`
  	LinkID      uint      `gorm:"index" json:"linkId"`
  	CreatedAt   time.Time `gorm:"index" json:"createdAt"`
  	IP          string    `gorm:"size:64" json:"ip"`
  	Country     string    `gorm:"size:64" json:"country"`
  	Region      string    `gorm:"size:128" json:"region"`
  	City        string    `gorm:"size:128" json:"city"`
  	Device      string    `gorm:"size:32" json:"device"`
  	Browser     string    `gorm:"size:64" json:"browser"`
  	OS          string    `gorm:"size:64" json:"os"`
  	Referer     string    `gorm:"type:text" json:"referer"`
  	UA          string    `gorm:"type:text" json:"ua"`
  	Fingerprint string    `gorm:"size:64;index" json:"-"`
  	IsBot       bool      `gorm:"default:false;index" json:"isBot"`
  	Variant     string    `gorm:"size:512" json:"variant"`
  	UTMSource   string    `gorm:"column:utm_source;size:128;index" json:"utmSource,omitempty"`
  	UTMMedium   string    `gorm:"column:utm_medium;size:128" json:"utmMedium,omitempty"`
  	UTMCampaign string    `gorm:"column:utm_campaign;size:128;index" json:"utmCampaign,omitempty"`
  }
  ```

### 2.4 Engine Components (`plugins/links/engine.go`)
- **clickItem** (`engine.go:58-78`):
  ```go
  type clickItem struct {
  	orgID       uint
  	slug        string
  	linkID      uint
  	ip          string
  	country     string
  	region      string
  	city        string
  	ua          string
  	device      string
  	browser     string
  	osStr       string
  	bot         bool
  	referer     string
  	fingerprint string
  	variant     string
  	utmSource   string
  	utmMedium   string
  	utmCampaign string
  	createdAt   time.Time
  }
  ```
- **Engine** (`engine.go:81-91`):
  ```go
  type Engine struct {
  	db          *gorm.DB
  	resolver    *origin.Resolver
  	ctx         *plugin.Context
  	queue       chan clickItem
  	wg          sync.WaitGroup
  	closeOnce   sync.Once
  	dropCount   atomic.Uint64
  	txCount     atomic.Uint64
  	rateLimiter *ipRateLimiter
  }
  ```
- **ipRateLimiter** (`engine.go:19-56`):
  Mutex-locked per-IP sliding window rate limiter (default: 300 req / minute window).

---

## 3. State Transitions & Execution Pipelines

```
[ Incoming Request /{slug} ]
              │
              ▼
   [ Cache Lookup: link:redirect:<host>:<slug> ]
         ├── (Hit Positive) ──► [ Expiry / Gate Check ]
         ├── (Hit Negative) ──► [ HTTP 404 ]
         └── (Miss) ──► [ Scoped DB Query (scopeForHost) ]
                              │
                    ┌─────────┴─────────┐
                    ▼                   ▼
            (Contested Host)    (Valid Link Found)
                    │                   │
               [ HTTP 404 ]             ▼
                              [ Cache Set (1h / 5s) ]
                                        │
                                        ▼
                           [ Expiry & Click Limit Check ]
                              ├── (Expired + ExpiredURL) ──► [ 302 to ExpiredURL ]
                              ├── (Expired + No ExpiredURL) ─► [ HTTP 404 ]
                              └── (Active)
                                        │
                                        ▼
                               [ Password Gate Check ]
                              ├── (Failed / Missing) ──► [ 401 HTML Form ]
                              └── (Passed)
                                        │
                                        ▼
                             [ Attribute Rule Match ]
                              ├── (Hit) ──► Override Target
                              └── (No Match)
                                        │
                                        ▼
                           [ Sticky A/B Split Hash ]
                           (sha256(fingerprint + linkID))
                                        │
                                        ▼
                             [ IP Rate Limit Check ]
                              ├── (Exceeded) ──► [ 302 Redirect (No Log) ]
                              └── (Allowed)
                                        │
                                        ▼
                         [ Non-Blocking Click Enqueue ]
                           (chan clickItem, cap 5000)
                                        │
                                        ▼
                             [ 302 Found Redirect ]
                                        │
                     ┌──────────────────┴──────────────────┐
                     ▼                                     ▼
           [ Batch Worker Flush ]              [ Client Browser Navigates ]
           (Every 100 items / 100ms)
                     │
                     ▼
           [ Quota Cap Check ]
           (plugin.CheckQuota)
           ├── (Exceeded) ──► Suppress Event & Increment
           └── (Allowed)  ──► Transactional INSERT &
                              Atomic UPDATE clicks
```

### 3.1 Link Creation & Validation Pipeline
1. **Scheme Validation**: Target URL scheme must be `http` or `https` (`helpers.go:19-28`). Bare hosts default to `https://`. Javascript/data URI schemes are strictly rejected.
2. **Redirect Targets Validation**: `ExpiredURL` and all `RoutingRule.Target` endpoints are validated and normalized via `validateRedirectTargets` (`helpers.go:37-53`).
3. **Host Ownership Gate**:
   - `p.ownsHost(orgID, host)` (`hosts.go:13-40`): Queries `dns.Domain` where `owner_id = orgID AND for_link = true`. Checks `EffectiveLinkHosts()`; falls back to domain apex `Domain.Name` if empty.
   - Refuses unauthorized hosts with HTTP 403 Forbidden.
4. **Multi-Tenant Shared Root Isolation**:
   - `p.linkHostRequired(orgID)` (`hosts.go:48-58`): If `models.BaseDomain` is configured and workspace owns at least one link host, empty host (`host = ""`) is rejected with HTTP 400 Bad Request to prevent cross-tenant collisions in the shared namespace.
5. **Slug Generation & Reservation**:
   - Empty slug generates random 6-character alphanumeric slug (`models.RandomSlug(6)`).
   - Custom slug verified against built-in reserved words (`admin`, `api`, `assets`, `portal`, `robots.txt`) and global setting `reserved_slugs` (`plugin.go:89-110`). Returns HTTP 409 Conflict if reserved.
6. **Quota Check**:
   - Evaluates `plugin.CheckQuota(ctx, orgID, "links", 1)` (`helpers.go:72-77`). Returns HTTP 402 Payment Required if plan lacks capability (`ErrQuotaUnavailable`), or HTTP 429 Too Many Requests if quota exhausted (`ErrQuotaExceeded`).
7. **Insertion & Side Effects**:
   - Inserts row (`Enabled: true, Archived: false, Clicks: 0`).
   - Audits `link.create`, publishes webhook `link.create`, enqueues background task `link.crawl` for title prefilling (`link_crawl.go`), and deletes cache key `link:redirect:<host>:<slug>`.

### 3.2 Slug Reservation & Retirement Cooldown
- When links are deleted, their cache entry is purged immediately (`crud.go:263`).
- When a link is purged or an organization deleted, slugs transition to retired quarantine status in `OrgSlugHistory` to prevent hostile takeover or recycling abuse.

### 3.3 Resolution & Redirection Pipeline
1. **Ingress**: `ctx.HandleRoot` captures path `/{slug}` (`routes.go:66-80`).
2. **Cache Resolution**: Checks `"link:redirect:" + host + ":" + slug`.
   - Positive hit returns `*Link`.
   - Negative hit (`Link.ID == 0`) returns HTTP 404 Not Found without hitting DB (`lookup.go:24-30`).
3. **Scoped Host Resolution (`scopeForHost`)** (`lookup.go:72-97`):
   - Queries `origin.Resolver.OwnerOf(host)`:
     - Single owner: queries `slug = ? AND (host = ? OR host = '') AND owner_id = owner`.
     - Contested host (`ServesTraffic(host)` is true but `OwnerOf` returns false): fails closed, returns `nil, false` (HTTP 404).
     - Unowned / shared host: queries only host-agnostic links `slug = ? AND host = ''`.
   - Orders by `host DESC` so exact host match wins over generic fallback.
   - Checks `linkHostDisabled`: if domain disabled for links, resolution fails (`lookup.go:51-53`).
   - Caches negative result for 1 minute (`lookup.go:101-106`).
   - Caches positive result for 1 hour; if `ClickLimit > 0`, TTL is reduced to 5 seconds (`clickLimitCacheTTL`, `lookup.go:12, 58`).
4. **Expiry & Click Limit Gate** (`handle.go:19-28`, `lookup.go:136-144`):
   - Evaluates `link.ExpiresAt != nil && now.After(*link.ExpiresAt)` and `link.ClickLimit > 0 && link.Clicks >= link.ClickLimit`.
   - If expired and `link.ExpiredURL != ""`: HTTP 302 Found redirect to `ExpiredURL` (with `Referrer-Policy: no-referrer`).
   - If expired and no `ExpiredURL`: HTTP 404 Not Found.
5. **Password Protection Gate** (`handle.go:29-38`):
   - If `link.Password != ""`, inspects `pw` from form body or query string.
   - Compares via `crypto/subtle.ConstantTimeCompare`.
   - If invalid/missing: responds with HTTP 401 Unauthorized rendering secure HTML password gate form (`handle.go:291-306`).
6. **Conditional Routing Rules** (`handle.go:58-81`):
   - Attribute rules (`geo`, `device`, `os`, `language`): evaluated first in sequential order. First match overrides `target`.
   - Split rules (A/B testing): evaluated ONLY if no attribute rule matched.
7. **Sticky A/B Split Assignment Algorithm (`splitAssign`)** (`handle.go:148-177`):
   - Pure function: `(fingerprint, linkID) -> (target, variant, ok)`.
   - Derives stable bucket: `h := sha256.Sum256([]byte(fingerprint + "\n" + fmt.Sprintf("%d", linkID)))`, `bucket := int(binary.BigEndian.Uint32(h[:4]) % 100)`.
   - Walks split rules, summing weights. First rule where `bucket < cum` wins.
   - Residual share (< 100) falls back to control (`variant = "control"`).
   - Sum > 100 is truncated. Never returns 500 error.
8. **IP Rate Limiting & Analytics Enqueue** (`handle.go:83-91`, `engine.go:218-254`):
   - `ipRateLimiter.Allow(ip)`: if exceeded, performs HTTP 302 redirect immediately without queuing analytics.
   - Extracts and anonymizes IP (IPv4 zeroed `/24`, IPv6 zeroed last 80 bits).
   - Generates 128-bit hex device fingerprint from anonymized IP, UA, and Accept-Language.
   - Extracts and truncates `utm_source`, `utm_medium`, `utm_campaign` to 128 chars.
   - Non-blocking enqueue to `queue chan clickItem` (cap 5000). On full, increments atomic `dropCount`.
   - Responds HTTP 302 Found to resolved target URL.

### 3.4 Asynchronous Batch Worker & Quota Capping (`engine.go:125-264`)
- Channel queue capacity: 5000 items.
- Flushing interval: 100 items or 100ms timer.
- Panic isolation: consumer wrapped in `safego.Recover("links.click-worker")`.
- Free-Tier Click Cap Enforcement:
  - Aggregates non-bot clicks by organization: `clicksByOrg[item.orgID]++`.
  - Evaluates `plugin.CheckQuota(ctx, orgID, "clicksPerMonth", count)`.
  - If `ErrQuotaExceeded`: marks org as `suppressed`. All clicks for this org skip both `link_events` insertion and `Link.clicks` increments.
  - Other quota errors fail open (permitted).
- Batch Persistence:
  - Single database transaction:
    1. Bulk insert `tx.Create(&events)`.
    2. Atomic CASE update: `UPDATE links SET clicks = clicks + CASE id WHEN ? THEN ? ... ELSE 0 END WHERE id IN ?`.
- Telemetry & EventBus:
  - Records usage via `ctx.RecordUsage(orgID, usagemetric.Clicks, count)` for unsuppressed clicks.
  - Publishes `link.click` webhook events.

---

## 4. Database Schema & Tenant Views

### 4.1 Table: `links` (`models.go:47-66`)
- `id` (uint, Primary Key, Auto Increment)
- `owner_id` (uint, Column: `owner_id`, Index: `idx_links_owner_id`, Default: 1)
- `host` (varchar(255), Composite Unique Index: `idx_host_slug`)
- `slug` (varchar(255), Composite Unique Index: `idx_host_slug`)
- `target` (text, Not Null)
- `password` (varchar(255))
- `note` (text)
- `title` (varchar(255))
- `tags` (varchar(512))
- `expires_at` (datetime / timestamp with time zone, Nullable)
- `expired_url` (text)
- `click_limit` (bigint, Default: 0)
- `archived` (boolean, Index: `idx_links_archived`, Default: false)
- `enabled` (boolean, Default: true)
- `routing_rules` (text, JSON-serialized `RoutingRules`)
- `clicks` (bigint, Default: 0)
- `created_at` (datetime)
- `updated_at` (datetime)

### 4.2 Table: `link_events` (`models.go:69-93`)
- `id` (uint, Primary Key, Auto Increment)
- `link_id` (uint, Index: `idx_link_events_link_id`)
- `created_at` (datetime, Index: `idx_link_events_created_at`)
- `ip` (varchar(64))
- `country` (varchar(64))
- `region` (varchar(128))
- `city` (varchar(128))
- `device` (varchar(32))
- `browser` (varchar(64))
- `os` (varchar(64))
- `referer` (text)
- `ua` (text)
- `fingerprint` (varchar(64), Index: `idx_link_events_fingerprint`)
- `is_bot` (boolean, Index: `idx_link_events_is_bot`, Default: false)
- `variant` (varchar(512))
- `utm_source` (varchar(128), Index: `idx_link_events_utm_source`)
- `utm_medium` (varchar(128))
- `utm_campaign` (varchar(128), Index: `idx_link_events_utm_campaign`)

### 4.3 Tenant Views & Redaction (`views.go:10-72`)
- **`tenant_links`**:
  - Definition: `SELECT id, owner_id, host, slug, target, password, note, title, tags, expires_at, expired_url, click_limit, archived, enabled, routing_rules, clicks, created_at, updated_at FROM links WHERE owner_id = %d`
  - Redacted Columns (`Sensitive`): `["password"]`
- **`tenant_link_events`**:
  - Definition: `SELECT le.id, le.link_id, le.created_at, le.ip, le.country, le.region, le.city, le.device, le.browser, le.os, le.referer, le.ua, le.fingerprint, le.is_bot, le.variant, le.utm_source, le.utm_medium, le.utm_campaign FROM link_events le INNER JOIN links l ON le.link_id = l.id WHERE l.owner_id = %d`
  - Redacted Columns (`Sensitive`): `["fingerprint"]`

---

## 5. Extension Seams & Integration Points

### 5.1 Services Provided (`lifecycle.go:63-74`)
| Service Coordinate (Wire String) | Interface / Type | Purpose |
|---|---|---|
| `plugin.OverviewServiceName("links")` (`"links.overview"`) | `plugin.OverviewFunc(p.overview)` | Workspace overview dashboard statistics |
| `plugin.PurgeServiceName("links")` (`"links.purge"`) | `plugin.PurgeFunc(p.purge)` | Complete data purge on organization deletion |
| `plugin.ExportServiceName("links")` (`"links.export"`) | `plugin.ExportFunc(p.exportData)` | Workspace JSON data export |
| `plugin.ServiceLinkResolve` (`"links.resolve"`) | `plugin.LinkResolver(p.resolveSlug)` | Maps (host, slug) to target & orgID for abuse moderation |
| `"links.create"` | `plugin.LinkCreator(p)` | Programmatic creation of short links across plugins |
| `plugin.CleanupServiceName("links")` (`"links.cleanup"`) | `plugin.CleanupFunc(p.cleanupEvents)` | Time-based batch retention pruning of old click events |
| `plugin.MCPExportServiceName("links")` (`"links.mcp_export"`) | `plugin.MCPExporter(p.mcpExportLinks)` | MCP-compatible JSON workspace data export |
| `"links.trust_proxy"` | `func(bool)` (`SetTrustProxy`) | Sets upstream proxy header trust configuration |

### 5.2 Declarative Dual Endpoint & MCP Tool
- **Declarative Endpoint**: `create_shortlink` (`POST /api/links/declarative`, `declarative.go:56-65`). Emits RFC7807 problem details on errors (`UNAUTHORIZED`, `HOST_REQUIRED`, `FEATURE_UNAVAILABLE`, `QUOTA_EXCEEDED`, `MISSING_DESTINATION`, `INVALID_DESTINATION`, `SLUG_RESERVED`, `SLUG_ALREADY_EXISTS`).
- **MCP Tool**: `list_links` (`mcp.go:44-85`), allows AI agents to inspect short link metrics.

### 5.3 Services Consumed from Core & Other Plugins
- `plugin.Context`:
  - `ctx.DB`, `ctx.UserID`, `ctx.OrgID`, `ctx.Audit`
  - `ctx.GetGlobalSetting`, `ctx.SetGlobalSetting`, `ctx.GetWorkspaceSetting`
  - `ctx.Enqueue`, `ctx.RegisterTask` (`link.crawl`)
  - `ctx.DeleteCache`, `ctx.CacheGet`, `ctx.CacheSet`
  - `ctx.PublishEvent`, `ctx.RegisterWebhookEvent` (`link.create`, `link.click`, `link.delete`)
  - `ctx.RequireRole`, `ctx.IsInstanceAdmin`
  - `ctx.RegisterTenantView`, `ctx.HandleRoot`, `ctx.Huma`
  - `ctx.GeoLookup`, `ctx.ParseUA`, `ctx.RecordUsage` (`usagemetric.Clicks`)
- `plugins/dns`: Domain verification and host ownership checking (`dns.Domain`, `EffectiveLinkHosts()`).
- `internal/origin`: `origin.Resolver` for host ownership and traffic serving resolution.
- `plugin.CheckQuota`: Metered resource quota enforcement for `"links"` and `"clicksPerMonth"`.

---

## 6. Invariant Guarantees & Error Behaviors

1. **Redirects Never Fail on Quota**: Reaching monthly click allowances suppresses logging and DB writes, but HTTP 302 redirects continue functioning indefinitely.
2. **Fail-Closed on Contested Domains**: If multiple tenants register the same host, `scopeForHost` fails closed and returns HTTP 404 rather than risking cross-tenant brand hijacking.
3. **Deterministic Sticky A/B Allocation**: Hashing fingerprint and link ID ensures identical variant routing on every repeat visit while eliminating cross-link bias.
4. **Non-Blocking Ingestion**: Channel queue overflow drops events with atomic counter increments; redirection latency is never degraded by database contention or queue saturation.
5. **Zero Binary Bloat Guarantee**: Technical specification `SPEC.md` resides in the plugin root and is strictly excluded from `//go:embed docs`, preventing binary inflation or leakage via public `/api/help/` endpoints.

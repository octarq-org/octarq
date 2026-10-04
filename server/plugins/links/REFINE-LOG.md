# Refinement Audit Log: `plugins/links`

## Module Summary
- **Module Path**: `octarq/server/plugins/links`
- **Priority**: P0 (Rank 14, In-Degree 1)
- **Refinement Date**: 2026-10-04
- **Branch**: `refine/pd-links`
- **Worktree**: `/Volumes/PHD/code/.worktrees/octarq-refine-links`

---

## Refinement Steps Executed

### 1. Stage 1: Characterization Locking
- Created `plugins/links/links_characterization_test.go` (145 LOC):
  - `TestLinks_Characterization_InterfaceContracts`: Locks compile-time interfaces (`plugin.Plugin`, `plugin.Describer`, `plugin.MenuProvider`, `plugin.InstanceMenuProvider`, `plugin.HelpDocsFS`, `plugin.Starter`, `plugin.LinkCreator`), plugin identity (`links`), and display metadata (`Short Links`).
  - `TestLinks_Characterization_MultiTenantIsolation`: Locks strict tenant row partitioning on `Link` and `LinkEvent` by `owner_id`/`org_id`, asserting that cross-tenant queries return empty sets and cross-tenant link deletions strictly fail.
  - `TestLinks_Characterization_FailClosedHostResolution`: Locks fail-closed host resolution (`ownsHost`) when two or more tenants contest a domain name, rejecting both tenants to prevent traffic hijacking.
  - `TestLinks_Characterization_UTMAndAttributionState`: Locks preservation of UTM query parameters (`utm_source`, `utm_medium`, `utm_campaign`) across link events.
  - `TestLinks_Characterization_ExpiredLinkState`: Locks expiration validation behavior for transient links.

### 2. Stage 2: Diagnostic Verification & Swallowed Errors Elimination
- Audited all 17 swallowed error locations in production Go code:
  - `crud.go:90`: Replaced `_ = p.enqueue(...)` with explicit error checking and structured warning log (`log.Printf`), and propagated `owner_id: l.OrgID` into the payload.
  - `crud.go:94`: Replaced `_ = p.deleteCache(...)` in `quickCreateLink` with structured warning log.
  - `crud.go:183, 185`: Replaced `_ = p.deleteCache(...)` on old and new hosts in `updateLink` with structured error logging.
  - `crud.go:233`: Replaced `_ = p.deleteCache(...)` in `deleteLink` with structured error logging.
  - `declarative.go:70`: Replaced `_ = p.deleteCache(...)` in `createDeclarativeLink` with structured warning log.
  - `export.go:41, 43`: Replaced `_ = cw.Write(...)` on header and CSV rows with explicit error handling and structured warning logging.
  - `links.go:225`: Replaced `_ = p.enqueue(...)` in `createLink` with structured warning logging, and included `owner_id: l.OrgID` in payload.
  - `links.go:229`: Replaced `_ = p.deleteCache(...)` in `createLink` with structured warning log.
  - `lookup.go:60`: Replaced `_ = e.ctx.CacheSet(...)` on positive link redirect lookup with structured warning log.
  - `lookup.go:104`: Replaced `_ = e.ctx.CacheSet(...)` on negative cache entries in `cacheNegative` with structured warning log.
  - `mcp_write.go:88`: Replaced `_ = p.deleteCache(...)` in `mcpCreateLink` with structured error log.
  - `mcp_write.go:169`: Replaced `_ = p.deleteCache(...)` in `mcpUpdateLink` with structured error log.
  - `mcp_write.go:218`: Replaced `_ = p.deleteCache(...)` in `mcpDeleteLink` with structured error log.
  - `routes.go:56`: Replaced `_ = plugin.RegisterEndpoint(...)` with explicit error check and structured warning log.
  - `routes.go:158`: Replaced `_ = p.host.Settings().SetGlobalSetting(...)` with structured warning log.
  - `service.go:38`: Replaced `_ = p.deleteCache(...)` in `CreateLink` with structured warning log.
  - `link_crawl.go:114`: Enforced `owner_id` scoping in `handleLinkCrawl` when updating page title to eliminate potential IDOR.
- **Diagnostic Result**: **0 swallowed errors remain** in production Go code across `plugins/links`.

### 3. Stage 3: Atomic Refactoring & Hygiene Guard (<500 LOC Hard Ceiling)
- Production files: All 20 production Go files were already strictly under 350 LOC.
- Test files: Decomposed 2 monolithic test God files (>500 LOC):
  - `gap_cover_test.go` (716 LOC) $\to$ decomposed into:
    - `gap_cover_mcp_test.go` (341 LOC): Page meta parsing, crawl handler, service, menus, and MCP tool tests.
    - `gap_cover_analytics_test.go` (386 LOC): Tag boundaries, referer classification, channel attribution, split routing, and resolve methods.
  - `handler_edge_test.go` (511 LOC) $\to$ decomposed into:
    - `handler_edge_crud_test.go` (241 LOC): List paging, org scoping, metadata parsing, and create edge branches.
    - `handler_edge_actions_test.go` (280 LOC): Quick create, update, delete, CSV export, link stats, and QR image verification.
- **Result**: **100% of production and test Go files** in `plugins/links` are strictly under 500 lines of code.

### 4. Stage 4: Multi-Tier Verification Loop
- `go test -race ./plugins/links/...`: **100% PASS** (9.88s, 0 race conditions).
- `go build ./...`: **100% PASS** across the entire `octarq/server` workspace.
- `gofmt -w plugins/links`: **CLEAN**.

### 5. Stage 5: Three-Tier Documentation Sync
- Living engineering specification verified: `plugins/links/SPEC.md` (22.2 KB, non-embedded).
- In-app bilingual help documentation verified: `plugins/links/docs/short-links.mdx` and `short-links.zh.mdx`.
- Compile-time interface assertions verified in `plugin.go` and `service.go`.

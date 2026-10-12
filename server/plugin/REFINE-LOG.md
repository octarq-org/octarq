# Refine Log: `octarq/server/plugin`

## 1. Module Profile Before Refinement
- **Path**: `octarq/server/plugin`
- **In-Degree**: 20 (Primary SPI Host and lifecycle engine for all plugins)
- **Production Files**: 29
- **Prod LOC**: 4,242 LOC
- **Test Files**: 25 (4,851 LOC)
- **Prior Symptoms**:
  - God file `plugin.go` (1,010 LOC) mixing SPI host definitions with Help Center document parsing and category data.
  - Swallowed panics and errors in `TypedBus.PublishAsync` (`_ = recover()`, `_ = fn(e)`).
  - Missing Tier 2 Engineering Specification (`SPEC.md`).

---

## 2. Refinement Actions Executed

1. **Characterization Lock**:
   - Authored `plugin_characterization_test.go` locking `TypedBus` event publishing, context propagation (`WithOrgID`/`WithUserID`), HelpDoc YAML frontmatter parsing and taxonomy normalization, and Service Registry `Provide`/`LookupAs`.
2. **Structural Decoupling (God File Elimination)**:
   - Decomposed `plugin.go` by extracting `HelpCategory`, `HelpCategories()`, `CompareHelpDocs()`, `HelpDocTranslation`, `HelpDoc`, `ParseHelpDoc()`, `ParseHelpDocSafe()`, `FillDefaults()`, and `MustParseHelpDoc()` into dedicated [`helpdoc.go`](file:///Volumes/PHD/code/.worktrees/octarq-refine-plugin/server/plugin/helpdoc.go).
   - Reduced `plugin.go` from 1,010 LOC down to 770 LOC (−240 lines, −24% size reduction) without altering any external exports or public signatures.
   - Cleaned up unused `gopkg.in/yaml.v3`, `log`, and `strings` imports in `plugin.go`.
3. **Observability & Error Tracing**:
   - Updated `TypedBus.PublishAsync` in `typed_bus.go` to log recovered panics and asynchronous handler error returns with event type details, eliminating silent event drop.
4. **Three-Tier Governance Alignment**:
   - Authored `server/plugin/SPEC.md` documenting plugin lifecycle state machine, AutoMigrate isolation rules, service registry conventions, and multi-tenant security invariants.

---

## 3. Verification Evidence

- `go test -v ./plugin/...` -> 100% PASS
- `go test -race ./plugin/...` -> PASS (Zero race conditions)
- `gofmt -w .` -> Clean
- `go build ./...` across `server/` -> Clean build

---

## 4. Follow-up: Residual God-File Split (2026-10-12)

The helpdoc extraction above left `plugin.go` at 770 LOC — still the last >500-LOC production file in the OSS server. Final split, pure moves within the package:

- `context.go` (new) — `Context`, `TenantDBFor`, `WithOrgID`/`OrgIDFromContext`/`WithUserID`/`UserIDFromContext`.
- `services.go` — absorbed the well-known service names + contracts the first pass missed: `ServiceDNSManager`/`DNSManager`/`DNSRecord`, `ServiceMailStorageProvider`/`StorageProvider`/`ErrStorageNotFound`, `ServiceQuotaChecker`/`QuotaChecker`/`ErrQuotaExceeded`/`ErrQuotaUnavailable`, `LinkCreator` (this file is what the package doc designates for well-known services).
- `identity.go` — absorbed `AuthMethod`, `ErrLoginRegistrationDisabled`, `ErrAccountLinkRequired`.
- `menu.go` (new) — `MenuItem`/`MenuProvider`/`InstanceMenuProvider`/`Action`/`ActionProvider`.
- `info.go` (new) — category constants, `ValidCategories`, `Info`/`Describer`/`Describe`/`FeatureKey`/`FeatureIsCore`.
- `events.go` (new) — `EmailEvent`/`WebhookEventDef`.
- `tracing.go` (new) — `StartSpan`.
- `plugin.go` — now 179 LOC: package doc + `Plugin`/`Mux`/`Starter`/`Validator` + optional-capability interfaces.

No behavior or API change: all symbols, signatures and doc comments preserved verbatim (content-conservation check; the surviving `plugin.go` diff is deletions-only). Stale `plugin/plugin.go:<line>` references in `scope_boundary_test.go` comments, the website architecture docs, and help/mail `SPEC.md` were re-pointed to symbols/files (the numbers had drifted with earlier edits).

Verification: `gofmt -l` clean; `go build ./...` and `go vet ./plugin/...` clean; `go test ./... -race` full suite green; largest new file 388 LOC.

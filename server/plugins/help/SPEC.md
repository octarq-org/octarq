# Help Plugin Specification (`plugins/help/SPEC.md`)

## 1. Architectural Overview & System Role

The `help` plugin (`octarq/plugins/help`) is the system-wide in-app documentation aggregator and API server:
1. **Core Capability**: Marked `Info.Core = true` (`help.go:57`) — always active, non-optional, and has no workspace toggle.
2. **Plumbing Aggregator**: Collects and normalizes documentation contributed by all active plugins across the instance.
3. **Dual Discovery Architecture**: Seamlessly aggregates documentation from the embedded directory convention (`HelpDocsFS`) and dynamic runtime providers (`HelpProvider`).
4. **Memoized FS Parser**: Caches parsed filesystem documentation (`docsFSCache`) keyed by plugin interface instance.
5. **Taxonomy Normalization**: Maps arbitrary plugin categories into the closed set of 6 standard help categories.
6. **Security-Hardened Markdown Pipeline**: Server-side goldmark rendering with strict HTML escaping (`html.WithUnsafe()` omitted).
7. **Starlight Aside Conversion**: Real-time conversion of Astro Starlight container directives (`:::tip`) into GitHub Flavored Markdown alerts (`> [!TIP]`).
8. **Platform-Level Documentation Custody**: Custodian of core non-plugin documentation (Auth, Orgs, RBAC, Tokens, Webhooks, Notifications, MCP).
9. **Dual Declarative Endpoints & Extension Seams**: Anchors core platform extension interfaces (EndpointSpec, TypedBus, AgentError).

- **Plugin Identifier**: `"help"` (`help.go:50`)
- **Plugin Metadata**:
  - Title: `"Help"`
  - Description: `"In-app documentation, versioned with the binary."`
  - Core: `true`
  - Category: `"footer"` (Sidebar menu)
- **Implemented Interfaces**:
  - `plugin.Plugin` (`help.go:36`)
  - `plugin.MenuProvider` (`help.go:37`)
  - `plugin.Describer` (`help.go:38`)
  - `plugin.HelpDocsFS` (`help.go:39`)
- **Database Tables**: None (`p.Models()` returns `nil`, `help.go:61`). Purely computational content aggregator.
- **Binary Embedding Guarantee**: Co-located at `plugins/help/SPEC.md`. Strictly excluded from binary embedding (`//go:embed docs` in `help.go:32-33` only bundles user help docs under `docs/`). Zero binary bloat, zero secret leak.

---

## 2. Core Go Types & Compile-Time Assertions

### 2.1 Plugin Struct & Interface Assertions (`help.go:35-44`)

```go
type Plugin struct {
	pctx *plugin.Context
}

var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
	_ plugin.HelpDocsFS   = (*Plugin)(nil)
)
```

### 2.2 Public Help Contracts (`octarq/plugin/`)

1. **`HelpDocsFS`** (`plugin/helpdocs_fs.go:26-28`):
   ```go
   type HelpDocsFS interface {
       HelpDocsFS() fs.FS
   }
   ```
2. **`HelpProvider`** (`plugin/plugin.go:922-924`):
   ```go
   type HelpProvider interface {
       HelpDocs() []HelpDoc
   }
   ```
3. **`HelpDoc`** (`plugin/plugin.go:804-812`):
   ```go
   type HelpDoc struct {
       Slug         string                        `yaml:"slug" json:"slug"`
       Title        string                        `yaml:"title" json:"title"`
       Category     string                        `yaml:"category" json:"category"`
       Order        int                           `yaml:"order" json:"order"`
       Feature      string                        `yaml:"feature" json:"feature,omitempty"`
       Markdown     string                        `yaml:"-" json:"markdown"`
       Translations map[string]HelpDocTranslation `yaml:"translations" json:"translations,omitempty"`
   }
   ```
4. **`HelpCategory`** (`plugin/plugin.go:683-688`):
   ```go
   type HelpCategory struct {
       Key    string            `json:"key"`
       Order  int               `json:"order"`
       Icon   string            `json:"icon"`
       Labels map[string]string `json:"labels"`
   }
   ```

---

## 3. Aggregation, Caching & Normalization Pipeline

### 3.1 Document Discovery & Caching Architecture (`help.go:186-222`)

```
[Client Request: GET /api/help/docs]
               │
               ▼
   [Iterate Active Plugins]
               │
   ┌───────────┴───────────────────────┐
   ▼                                   ▼
[Plugin implements HelpDocsFS]    [Plugin implements HelpProvider]
   │                                   │
   ▼                                   ▼
[Lookup docsFSCache (sync.Map)]   [Invoke hp.HelpDocs()]
   │ (Key: plugin.Plugin value)        │
   ├─ Hit ──► Return cached docs       │
   └─ Miss ─► LoadHelpDocs(fsys)       │
              Store in docsFSCache     │
   │                                   │
   └───────────┬───────────────────────┘
               ▼
   [Concatenate All Documents]
               │
               ▼
   [Tenant Feature Gate Check] ──(Feature Inactive)──► Discard Doc
               │ (Active)
               ▼
   [Slug Collision Resolution] ──(Collision)─────────► Prefix "<plugin>-<slug>"
               │
               ▼
   [Category Taxonomy Normalization (FillDefaults)]
               │
               ▼
   [Language Matching (Exact -> Primary Subtag)]
               │
               ▼
   [Sort by Category Order -> Doc Order -> Title]
```

### 3.2 HelpDocsFS Traversal Mechanics (`plugin/helpdocs_fs.go:48-96`)

When `pluginDocs` encounters a cache miss for a `plugin.HelpDocsFS` provider, it invokes `plugin.LoadHelpDocs(fsys fs.FS)`:

1. **Recursive Filesystem Walk (`fs.WalkDir`)**:
   - Recursively walks `fsys` starting from root (`"."`).
   - Directories (`d.IsDir()`) are skipped.
   - Read or walk errors on individual entries are logged as warnings and skipped, ensuring one malformed file does not prevent the rest of the documentation corpus from loading.
2. **Extension & Language Suffix Discrimination (`helpDocBase`, lines 98-117)**:
   - Validates file extensions against recognized types `helpDocExts = []string{".mdx", ".md"}`. Non-matching files are skipped.
   - Trims extension to determine candidate `base`.
   - Checks whether `base` contains a translation language suffix matching the closed set `helpDocLangs = []string{"zh", "es", "pt", "ja"}` via `isHelpDocLangSuffix`.
   - **Translation Suppression**: Suffix-matched files (e.g. `webhooks.zh.mdx`) return `ok = false` and are omitted from the primary walk loop. This guarantees translated pages never create duplicate top-level navigation entries in the sidebar.
   - **Dotted Name Preservation**: Files with dots not in `helpDocLangs` (e.g. `api.v2.mdx`) return `ok = true` with `base = "api.v2"`, preserving versioned slugs as canonical English documents.
3. **Parsing & Fallback Semantics (`helpdocs_fs.go:67-80`)**:
   - Parses YAML frontmatter and body via `ParseHelpDocSafe(string(raw))`.
   - **Slug Fallback**: If frontmatter `doc.Slug` is empty, defaults to `base`. Subdirectory paths (e.g. `docs/nested/deep.md`) are ignored for slug generation, allowing directories to organize files on disk without altering public URL slugs.
   - **Title Fallback**: If frontmatter `doc.Title` is missing, logs a warning and falls back to `doc.Slug`.
4. **Sibling Translation Discovery & Attachment (`readHelpDocTranslations`, lines 81-85, 133-151)**:
   - For each canonical English document in directory `dir := path.Dir(p)`, probes for sibling files matching `<base>.<lang>.<ext>` across all configured languages (`helpDocLangs`: `zh`, `es`, `pt`, `ja`) and extensions (`.mdx`, `.md`).
   - Reads the raw content of the first matching extension for each language and attaches it via `doc = doc.WithTranslation(lang, raw)` (`plugin.go:816-835`), storing parsed translation title, category, and markdown in `doc.Translations[lang]`.
5. **Deterministic Sort Ordering (`CompareHelpDocs`, line 94; `plugin/plugin.go:776-794`)**:
   - Sorts loaded documents prior to cache storage using `sort.Slice(docs, func(i, j int) bool { return CompareHelpDocs(docs[i], docs[j]) })`.
   - Documents are ordered hierarchically by:
     1. Category taxonomy order: `helpCategoryMap[a.Category].Order < helpCategoryMap[b.Category].Order` (unmapped categories default to order `9999`).
     2. Document order: `a.Order < b.Order`.
     3. Document title: `a.Title < b.Title` (lexicographical).

### 3.3 Cache Key Stability Guarantee
- `docsFSCache` (`sync.Map`) is keyed by the `plugin.Plugin` interface value, **not** by `pl.Name()`.
- Rationale: Downstream commercial distributions mount both the OSS help plugin and a Pro help extension module, both answering `Name() == "help"` so they share a single UI toggle. Keying by name caused cache collisions where one overwritten the other. Interface pointer identity guarantees zero cross-module cache corruption.

### 3.4 Closed Category Taxonomy (`plugin/plugin.go:690-766`)
The platform enforces a closed set of 6 standard categories:
1. `start` (Order: 10, Icon: `"book-open"`): "Start & platform" / "入门与平台"
2. `operations` (Order: 20, Icon: `"workflow"`): "Operations" / "运营"
3. `infrastructure` (Order: 30, Icon: `"boxes"`): "Infrastructure" / "基础设施"
4. `commerce` (Order: 40, Icon: `"wallet"`): "Commerce" / "商务"
5. `settings` (Order: 50, Icon: `"settings"`): "Settings" / "设置"
6. `licensing` (Order: 60, Icon: `"key"`): "Editions & licensing" / "版本与授权"

`FillDefaults` automatically remaps legacy categories (`getting-started` -> `start`, `links`/`mail`/`dns` -> `services` -> fallback to `operations`/`infrastructure`).

---

## 4. Markdown & Aside Conversion Engine (`markdown.go`)

### 4.1 Starlight Aside Directive Rewriting (`markdown.go:20-93`)
The documentation corpus is authored once and consumed by both Astro Starlight and the in-app help viewer. The engine translates Starlight container directives into GitHub Flavored Markdown blockquotes:

| Starlight Input | Rewritten GFM Alert | Notes |
|---|---|---|
| `:::note` | `> [!NOTE]` | Standard callout |
| `:::tip` | `> [!TIP]` | Best practices & guidance |
| `:::caution` | `> [!WARNING]` | Warnings & cautions |
| `:::warning` | `> [!WARNING]` | Warnings |
| `:::danger` | `> [!CAUTION]` | Mapped to highest available GFM severity |
| `:::important` | `> [!IMPORTANT]` | Critical prerequisites |
| `:::tip[Custom Title]` | `> [!TIP]`<br>`> **Custom Title**`<br>`>` | Custom title preserved as bold first line |

- **Fenced Code Immunity Scope & Asymmetric Nesting Limitation (`markdown.go:58-69`)**:
  - **Top-Level Code Block Protection**: Code fence tracking is strictly gated by `!inAside` at `markdown.go:58`. When outside an aside callout, encountering a code fence (` ``` ` or `~~~`) sets `fence = trimmed[:3]`. All subsequent lines within that fence are passed through verbatim, preventing top-level code blocks or scripts containing `:::` from triggering aside callout conversions.
  - **Nested Code Block Edge Case**: Because `markdown.go:58` requires `!inAside`, code blocks nested *inside* an active aside (`inAside == true`) do not activate the `fence` tracking variable. Consequently, code fences inside an aside do not inhibit `:::` closer evaluation. If a code snippet nested inside an aside contains a line matching `:::`, the aside closer condition (`trimmed == ":::"` at `markdown.go:65`) evaluates to true, prematurely resetting `inAside = false` and ending the blockquote. Subsequent lines of that code snippet leak into the output as unquoted markdown outside the alert blockquote.

### 4.2 Security-Hardened Markdown Rendering (`markdown.go:117-129`)
- Renders via Goldmark with GFM extensions and auto heading IDs.
- **Fail-Closed Security**: `html.WithUnsafe()` is deliberately omitted. All raw HTML in document markdown is escaped, ensuring user or third-party plugin documentation cannot inject cross-site scripting (XSS) attacks.

---

## 5. Platform-Level Documentation Custody

The `help` plugin serves as custodian for capabilities owned directly by core subsystems (`internal/` and `app/`) that lack plugin directories:
- `api-tokens.{mdx,zh.mdx}`: API token creation, scopes, and revocation.
- `authentication.{mdx,zh.mdx}`: Password policy, sessions, and recovery.
- `getting-started.{mdx,zh.mdx}`: First-run orientation and dashboard basics.
- `mcp.{mdx,zh.mdx}`: Model Context Protocol configuration and tool discovery.
- `multi-org.{mdx,zh.mdx}`: Multi-organization switching and tenant boundaries.
- `notifications.{mdx,zh.mdx}`: Notification channels (Telegram, webhooks).
- `quickstart.{mdx,zh.mdx}`: End-to-end setup walk-through.
- `webhooks.{mdx,zh.mdx}`: Inbound and outbound webhook triggers.

---

## 6. Core Platform Extension Seams

The `help` module anchors and coordinates core developer extension surfaces:

### 6.1 Declarative Dual Endpoints (`plugin/endpoints.go`)
- Enables a single Go struct to declare both an HTTP REST route and an MCP AI agent tool:
  ```go
  type EndpointSpec[In any, Out any] struct {
      Name            string
      Summary         string
      Description     string
      Method          string
      Path            string
      RequireAuth     bool
      RequireRole     []string
      RiskLevel       string // "read" | "write" | "destructive"
      RequireApproval bool   // HITL gate
      ExposeMCP       bool
      Handler         HandlerFunc[In, Out]
  }
  ```

### 6.2 Typed EventBus (`plugin/typed_bus.go`)
- Thread-safe in-memory event bus providing synchronous `Publish` and panic-isolated `PublishAsync`:
  ```go
  type TypedBus[T any] struct { ... }
  func (b *TypedBus[T]) Subscribe(h func(T) error)
  func (b *TypedBus[T]) Publish(e T) []error
  func (b *TypedBus[T]) PublishAsync(ctx context.Context, e T)
  ```

### 6.3 Agent-Native RFC7807 Problem Details (`plugin/errors.go`)
- Structured error standard communicating directly with AI Agents:
  ```go
  type AgentError struct {
      HTTPCode      int
      Code          string
      Message       string
      AgentGuidance string
      Retryable     bool
  }
  ```
- Methods: `ToProblem(instance)` generates RFC 7807 JSON; `FormatMCPAgentError(err)` formats MCP tool results.

### 6.4 Dynamic Plugin Configuration Schemas (`plugin/config_schema.go`)
- Strongly-typed plugin configuration schema (`ConfigSchema` and `ConfigField`).
- Generates Draft-07 JSON Schema with `x-secret: true` tagging for sensitive API credentials.

---

## 7. Invariant Guarantees & Fail-Closed Behaviors

1. **Always-On Core Invariant**: `help` is declared `Core: true`, guaranteeing documentation availability regardless of workspace toggle states.
2. **XSS Protection**: Markdown rendering escapes all raw HTML (`html.WithUnsafe()` omitted), protecting against malicious script injection in user-supplied or community docs.
3. **No Cross-Plugin Cache Collision**: Caches parsed FS documents using `plugin.Plugin` interface identity instead of string names, avoiding clashes between core plugins and Pro overrides.
4. **Tenant Isolation**: Only documents associated with active features for the querying organization are returned.
5. **Zero Binary Bloat**: `SPEC.md` resides in the plugin root and is strictly omitted from binary compilation (`//go:embed docs`), preserving security boundaries.
6. **Deterministic Translation Binding**: Primary filesystem walking filters out files matching `helpDocLangs` (`zh`, `es`, `pt`, `ja`), guaranteeing that localized files attach strictly as sibling translations (`Translations[lang]`) to canonical documents rather than registering as duplicate top-level navigation pages.

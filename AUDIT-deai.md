# AUDIT-deai —— 注释/文档去 AI 味儿清理报告

按 `deai-rulebook.md` 判定，只动 Go 注释与 Markdown 文档正文，未改任何 Go 逻辑。
本仓库注释质量整体极高（大量「为什么 / 契约 / 陷阱」注释），清理以「复述式
步骤标签」为主，密度很低。

## 改动统计

- **19 个文件**，净删 **57 行**、净增 **6 行**（其中 3 行是把 internal/api/overview.go
  的三行设计叙述压成两行约束说明）。
- Go 注释删除 38 行、文档删除 19 行。
- `git diff -- *.go` 除注释外无任何代码行变动（已逐行核验）。

### Go 注释清理明细（全部为「复述代码在干什么」的步骤标签）

| 文件 | 行 | 删掉的内容 |
| --- | --- | --- |
| `plugins/mail/mail.go` | 547/554/564/567/570/600/605/618 | `wrapLinksInEmail` 内 8 个复述标签（"Determine the short link domain host"、"Create the link record" 等），共 8 行 |
| `plugins/mail/webhook.go` | 273/465/520/541/576 | "Trigger Webhook Event Bus"、"Write Audit Log"、"Send alert (notifications)" 等 5 个标签 |
| `plugins/links/shortlink.go` | 316/348/561/572-573/577 | "Try reading from cache first"、"Collect split rules." 等 6 行（splitAssign 内 3 个标签与函数文档重复，函数文档已保留算法与"混入 linkID"的原因） |
| `internal/auth/auth.go` | 311/317/325 | "Try fetching from cache first"、"If expired, clean it up"、"Cache the retrieved session" |
| `internal/api/account.go` | 86 | "Redact webhook secrets."（上一段同样的 channel 脱敏本就没有注释） |
| `internal/api/auth.go` | 665 | "Check if another account already uses this email" |
| `internal/api/recovery.go` | 223 | "Invalidate all active sessions for this user" |
| `internal/api/tenant_menu.go` | 390/413 | "Find or create the target User."、"Check if already a member." |
| `internal/mcp/mcp.go` | 124 | "Register any plugin-supplied MCP tools." |
| `internal/cache/cache.go` | 35 | "Quick ping to check if Redis is up" |
| `internal/cleanup/cleanup.go` | 79/86 | "Expired sessions"、"Audit logs older than the retention window" |
| `internal/queue/queue.go` | 185 | "Shut down workers gracefully on context done" |
| `plugins/dns/providers.go` | 200 | "Check if any domain is using this account" |
| `plugins/dns/domains.go` | 485 | "Apply master switches when explicitly provided." |
| `internal/api/overview.go` | 61-63 | 3 行压成 2 行（保留重复键覆盖 + 告警的契约说明，去掉复述首行与自我论证） |

### 文档清理明细

| 文件 | 改动 |
| --- | --- |
| `README.md` | 示例 UIPlugin 里删掉不存在的 `menu:` 字段（1 行） |
| `README_ZH.md` | "控制面板"→"管理后台"×2（术语统一）、删"极致"、示例删 `menu:` 字段 |
| `website/src/content/docs/writing-a-plugin.md` | 示例删掉 UIPlugin 里不存在的 `menu:` 块（9 行） |
| `website/src/content/docs/guides/why-i-rewrote-a-saas-stack-as-a-go-plugin-framework.md` | 删"Starts in under 15ms / 30MB RAM"（无任何 benchmark 支撑的假数据断言）；删 Summary 两段空洞升华句（"best of both worlds"、"own your stack"），保留链接列表 |

## 「我犹豫过但决定保留」清单（比清理本身更重要，请复核）

这些按 rulebook 第 D 节「判断法」判为**保留**，但确实带一点「AI 味儿」特征：

| 位置 | 内容 | 为什么保留 |
| --- | --- | --- |
| `internal/models/models.go:51` | `// Session revocation is handled by deleting Session rows from user_sessions.` | 孤儿设计注记，正文与 Session 结构体文档重复；但删掉会丢掉"User 上没有 revoked 标记"这个指针，且位于结构体字段之间、无对应代码，删错风险大于收益 |
| `plugins/mail/mail.go:573` | `// Clean trailing punctuation that might be captured by regex in plain text` | 半复述半解释；"might be captured by regex" 是循环存在的原因，保留 |
| `plugins/mail/mail.go:591` | `// Skip if it is already our short link host or is localhost/internal` | 标识条件分支意图（三条件合写），删了要读两行代码才能确认目的 |
| `plugins/links/links.go:592` | `// Ensure the link belongs to the caller's org before exposing its analytics.` | 复述式，但点明了"暴露 analytics 前"这个越权防护意图，属安全上下文 |
| `internal/api/tenant_menu.go:87` | `// Verify the user belongs to the target organization.` | 复述式，但这是租户隔离检查点标记，误删会降低后续维护者对隔离边界的警觉 |
| `plugins/dns/plugin.go:5-8` | 包文档 "It was lifted out of the monolithic internal/api.Handler..." | 变更史味道，但同句内含有当前事实（走 plugin 契约、标记 Core、仍共享 internal/models 模型），压缩需重写整句，按「只删噪音不做文风改造」保留 |
| `README.md:30` / `README_ZH.md:30` | "Octarq isn't a URL shortener that also does email. It's a **framework**." | rulebook A4 的对仗句式（It's not A — it's B），但这是产品定位的核心句，改写风险大于收益 |
| `README.md:51` / `README_ZH.md:51` | "The point isn't 'we added AI.' The point is the **framework** wiring:" | 同上，定位句 |
| `README_ZH.md:34` | `## 开箱即用（官方参考插件）` | 「开箱即用」在 rulebook 空洞形容词列表，但此处单次使用且描述属实（默认构建确实内置这些插件），不算滥用 |
| `internal/server/middleware.go:223-225, 269-271` | `// Client IP` / `// Request ID` 段分隔线 | rulebook 明确允许既有分隔线保留（「分隔本身可以留，但不要新增」），这两处是既有结构不是新增 |
| `plugins/mail/webhook.go:94-95` | "Auth & Tenant check: ... return generic 401 ... so no org existence is leaked" | 半复述半安全说明（401 而非 404 防枚举），保留 |

## 文档承诺了但代码里不存在 / 与代码不符的清单（未改代码去实现）

| 位置 | 问题 |
| --- | --- |
| `website/src/content/docs/guides/why-i-rewrote-a-saas-stack-as-a-go-plugin-framework.md` §2 | 展示的 `plugin.Plugin` 接口带 `Init(ctx, pctx) error` 且 `Mount(router, pctx) error`；实际 `plugin/plugin.go` 的接口是 `Name()/Models()/Mount(mux Mux, ctx *Context)`，无 Init，Mount 无返回值 |
| 同上 §2 | `RegisterNotifier("slack", func(ctx, cfg string, msg notify.Message) error)` 回调签名与 `plugin.Context.RegisterNotifier(typ string, send func(ctx, cfgJSON, text string) error)` 不符，且 `notify.Message` 是 internal 包，插件无法引用（示例为示意性伪代码） |
| 同上 §3 | "building every HTTP route using **Huma v2**" 不准确：`/admin` SPA、`/{slug}` 重定向、部分 webhook 走标准库 mux，并非每个 HTTP 路由都经 Huma |
| 同上（已删） | "Starts in under 15ms and consumes under 30MB of RAM" —— 仓库里没有任何 benchmark 或测量代码支撑（已作为假数据断言删除） |
| `website/src/content/docs/architecture/composition.md:54-57` | 示例 `builtin.Default()` 只返回 dns/links/mail 三个；实际 `plugins/builtin/builtin.go` 返回 dns/links/mail/help 四个 |
| `docs/AUDIT-public-endpoints.md:43` | 引用 `plugins/dns/ddns.go`，该文件不存在；实际拆成 `plugins/dns/ddns_crud.go`、`ddns_update.go`、`ddns_secret.go` |
| `README.md:95`、`README_ZH.md:95`、`website/src/content/docs/writing-a-plugin.md:94-103` | 示例 UIPlugin 里有 `menu:` 字段；`@octarq/plugin-sdk` 的 `UIPlugin` 类型（`packages/plugin-sdk/src/contract/types.ts:119-126`）没有该字段，侧边栏入口只来自 Go 侧 `Menus()`（`examples/plugin-hello/web/index.ts` 有注释明确此点）。**已在文档中删除该字段**（见改动明细） |
| `website/src/content/docs/guides/why-i-rewrote-a-saas-stack-as-a-go-plugin-framework.md` 全文 | 与 `docs/PLUGINS.md`、`docs/PLUGIN-ARCHITECTURE.md` 相比语气明显更营销化（"Data Sovereignty"、"Own the Stack" 标题），但内容主体与代码相符，仅列上述条目 |

## 纪律符合性

- 未动 `web/**`、`CHANGELOG.md`、`.changeset/**`、任何 `_test.go`、任何 Go 逻辑。
- 未触碰 `config/`、`llmprovider/`、`openapi/`、`webembed/`、`examples/`（不在本线范围）。
- 验证全绿（见下）。

## 验证结果

```bash
gofmt -l .            # 无输出
go build ./...        # exit 0
go vet ./...          # exit 0
go test ./... -race   # 35 个包全部 ok，exit 0
```

# G-docs-consolidate 报告

仓库根 `docs/` 已全部移除。文档收敛到两个合法面:

- **应用内帮助** `plugins/*/docs/*.mdx` —— 随功能走,help 系统在 `/help/<slug>` 读取
- **website 文档站** `website/src/content/docs/` —— 部署、架构、插件编写、指南

---

## 1. 每个根 docs 文件的处置表

| 根 docs 文件 | 独有内容(website 没有的) | 处理方式 | 理由 |
|---|---|---|---|
| `docs/ACCESSIBILITY.md` | 审计行号引用(`worktree-agent-*` 分支)、§3 行级 FAIL 清单、§4 组件级 REC 建议、§5 reduced-motion 状态 | **删除**;唯一可公开的规范(对比度表、键盘/ARIA 要求)已 100% 存在于 `website/src/content/docs/guides/accessibility.md`;补了 website 缺的 reduced-motion 事实(一行) | 这是一次性审计工作记录,行号引用已过期,不是用户文档;可操作的规范已被 website 指南承载 |
| `docs/AUDIT-public-endpoints.md` | 整份(无 website 对应) | **移到仓库外** `.agy-specs/AUDIT-public-endpoints.md`(见 §2 复核) | 一次性审计报告,非文档 |
| `docs/PLUGIN-ARCHITECTURE.md` | §9 "Status & next steps" 交接状态 | **删除**;§1–§8 是 `website/src/content/docs/architecture/overview.md` 的近逐段副本 | §9 是内部 handoff 状态,所有 ✅ 项均已落地,过期且不属于公开文档 |
| `docs/PLUGINS.md` | 后端规则(404 auto-gate、402 license-gate、不 import internal、编译期断言、Own your tables)、前端规则(React.lazy、PluginGate、category=group、settings 路径、requiredRole advisory、i18n namespace)、服务注册懒解析、帮助文档契约(HelpDocsFS / `docs/<slug>.mdx` / `.zh.mdx` / HelpCategories 六键)、信任模型、分发、checklist | **删除**;全部压缩迁入 `website/src/content/docs/writing-a-plugin.md`(新增 §6–§9 + 后端/前端 Key rules) | 均有价值且已核实与代码一致(HelpDocsFS、octarq.plugins.json、HelpCategories 均存在) |
| `docs/PRE-LAUNCH.md` | 整份(无 website 对应) | **移到仓库外** `.agy-specs/PRE-LAUNCH.md`,内容原样保留(含刚更新的备份/恢复条目) | 内部上线清单,非公开文档 |
| `docs/PUBLISHING.md` | escape hatch(`sdk-v*` tag)、root scripts、secrets & permissions、loop-safe 说明、"No changeset = no release" | **删除**;压缩迁入 `website/src/content/docs/guides/publishing.md` | 均有价值且已核实(代码: `.github/workflows/publish-sdk.yml` 有 `sdk-v*` tag job、root `package.json` 有三个 scripts) |

## 2. `AUDIT-public-endpoints.md` 复核结果

**无未鉴权端点缺口。** 没有发现文档未列出的公开端点。

- 文档表格(20 项 method+path)与 `internal/api/public_endpoints_test.go` 的 `registeredPublicEndpoints` 逐一比对,**完全一致**。
- 代码的豁免机制(`internal/api/public_endpoints.go`:`publicExactPaths` 13 项 + `publicSubtreePrefixes` 仅 `/api/webhook/`)与文档描述吻合。
- 守护测试仍在: `TestPublicEndpointRegistry`、`TestPublicPrefixesAreNarrow`、`TestLogoutAllIsNotExemptByPrefix`、`TestPublicGETMatcher` 逻辑未变(仅注释里对文档的引用改为"kept outside this repo")。
- 小差异(非安全): 文档表格 "Exempt via" 列把 `GET /api/auth/methods` 标为 `metadata`,代码里它是 `publicExactPaths` 条目;webhook 三条文档标 `metadata`,实际走 `/api/webhook/` 子树。机制标注过时,不影响清单与结论。
- `/api/mcp/sse`、`/api/mcp/stream` 挂在自己的 `mcpAuth` 中间件下(会话 cookie / Bearer token / `?token=`),不属于"未经鉴权暴露",不在公开清单内是设计使然。

已移动: `docs/AUDIT-public-endpoints.md` → `/Volumes/PHD/code/.agy-specs/AUDIT-public-endpoints.md`(仓库外)。

## 3. 帮助文档 vs website 事实冲突清单

只报事实冲突,两侧给 `file:line`。

### 高优先级:一方描述的能力与代码不符

1. **OAuth 回调 URL 帮助文档写错**
   - 帮助文档: `plugins/help/docs/authentication.mdx:33` —— 回调 URL 为 `https://your-octarq-domain.com/api/auth/google/callback`
   - 代码: `internal/api/api.go:226` 注册 `GET /auth/callback/{provider}`;`internal/auth/oauth.go:165` 构造 `base + "/auth/callback/" + provider` → 实际是 `https://host/auth/callback/google`(**无 `/api/` 前缀**)
   - website: `website/src/content/docs/configuration.md:61` 写 `https://<host>/auth/callback/<provider>` —— **与代码一致**
   - 结论: 帮助文档的 URL 代码里不存在;website 正确。改哪边是产品决策,本批次未改。

2. **RAW 入站端点认证方式帮助文档写反**
   - 帮助文档: `plugins/mail/docs/mailboxes.mdx:18` —— raw 端点为 `/api/webhook/{orgSlug}/email/inbound/raw`(无 token 段),用 header `X-Octarq-Token` 或 `Authorization: Bearer` 认证
   - 代码: `plugins/mail/plugin.go:192` 注册 `POST /api/webhook/{orgSlug}/email/inbound/raw/{token}` —— **token 是路径段**;`plugins/mail/inbound_auth_test.go:72-88` 明确断言 header token 必须被拒("a correct token in a header must not authenticate — the path segment is the credential")
   - website: `website/src/content/docs/core/mailboxes.md` 未提及 raw 端点(无覆盖,无冲突)
   - 结论: 帮助文档描述的 header 认证路径代码里不存在且被测试钉死为拒绝。高优先级(文档描述的能力代码里没有)。本批次未改。

3. **保留 slug 前缀两边都与代码不符**
   - 帮助文档: `plugins/links/docs/short-links.mdx:31` —— 保留 `/admin`、`/api`、`/assets`、`/portal`、`/help`
   - 代码: `plugins/links/plugin.go:364` `builtinReservedSlugs = {admin, api, assets, portal}` —— **没有 `help`**
   - website: `website/src/content/docs/core/short-links.md:11` —— "every path that isn't `/admin` or `/api` is a potential slug" —— 漏了 `assets`/`portal`
   - 结论: 帮助文档多列了代码不保留的 `/help`;website 少列了代码保留的 `assets`/`portal`。两侧都需对齐代码。本批次未改。

### 低优先级(website 声称的扩展点代码中无实现,但为将来式措辞)

4. `website/src/content/docs/core/dns.md:12` —— "Cloudflare and DNSPod today, with Aliyun and Route53 slotting into the same interface"。
   代码 `plugins/dns/` 只有 `cloudflare`/`dnspod`(`plugins/dns/models.go:16`、`plugins/dns/plugin.go:83`),没有 aliyun/route53 实现。句子是将来式("slotting into"),不构成现成能力谎言,但严格说代码中不存在该扩展点。帮助文档 `dns.mdx` 只说 Cloudflare/DNSPod,与代码一致。

### 无冲突(已核实一致)

- DDNS 端点/哈希: `ddns.mdx:18,27` ↔ 代码 `plugins/dns/ddns_secret.go:43`(SHA-256)、`/api/dns/ddns/update` GET+POST —— 一致
- API token 格式: `api-tokens.mdx:12`("oct_ + 24 随机字节") ↔ `internal/api/tokens.go:21` —— 一致
- MCP: `mcp.mdx:13`(`/api/mcp/sse`、SSE 禁 SQL)↔ `internal/api/api.go:233`、`internal/mcp/networked_guard_test.go` —— 一致
- reset 链接 1 小时: `authentication.mdx:25` ↔ `internal/api/auth.go:344` —— 一致
- 密码重置登出所有会话: `authentication.mdx:26` ↔ `internal/api/auth.go:298`(bump SessionEpoch)—— 一致
- 邀请链接 24 小时: `multi-org.mdx:33` ↔ `internal/api/auth.go:749` —— 一致
- 入站 webhook 路径: `mailboxes.mdx:12` ↔ `plugins/mail/plugin.go` 注册路径 —— 一致

## 4. 引用的修改与 gif 的新位置

**gif**: `docs/assets/octarq-demo.gif` → **`assets/octarq-demo.gif`**(仓库根)。选择根 `assets/` 而非 `.github/assets/`: `.github/` 语义上属于 GitHub 组织/CI 配置,README 顶部 hero 媒体放根 `assets/` 是通用惯例,且该目录独立于任何文档面。两个 README 的路径已同步(`README.md:16`、`README_ZH.md:16`)。

**代码注释引用**(全部改为指向 website 对应位置):

| 文件:行 | 原引用 | 改为 |
|---|---|---|
| `app/app.go:280` | docs/PLUGIN-ARCHITECTURE.md | website/src/content/docs/architecture/overview.md |
| `internal/api/api.go:255` | docs/PLUGIN-ARCHITECTURE.md | website/src/content/docs/architecture/overview.md |
| `internal/api/helpers.go:195` | docs/PLUGIN-ARCHITECTURE.md | website/src/content/docs/architecture/overview.md |
| `internal/api/api_test.go:27` | docs/PLUGIN-ARCHITECTURE.md | website/src/content/docs/architecture/overview.md |
| `internal/api/public_endpoints_test.go:11` | docs/AUDIT-public-endpoints.md | "kept outside this repo"(AUDIT 已移出) |
| `plugins/dns/plugin.go:7` | docs/PLUGIN-ARCHITECTURE.md | website/src/content/docs/architecture/overview.md |
| `plugins/help/help.go:21` | docs/PLUGINS.md | website/src/content/docs/writing-a-plugin.md |
| `examples/edition-nomail/exclude_test.go:14` | docs/PLUGIN-ARCHITECTURE.md | website/src/content/docs/architecture/overview.md |
| `web/src/App.tsx:440` | docs/PLUGINS.md | website/src/content/docs/writing-a-plugin.md |
| `web/src/shell/areaForCategory.test.ts:7` | docs/PLUGINS.md | website/src/content/docs/writing-a-plugin.md |
| `web/src/shell/areas.tsx:172` | docs/PLUGINS.md | website/src/content/docs/writing-a-plugin.md |
| `web/src/shell/AreaPanel.tsx:264` | docs/PLUGINS.md | website/src/content/docs/writing-a-plugin.md |
| `web/src/shell/navHardcoding.test.ts:3` | docs/PLUGINS.md | website/src/content/docs/writing-a-plugin.md |

**文档/README 链接**:

| 文件:行 | 修改 |
|---|---|
| `.changeset/README.md:23` | docs/PUBLISHING.md → website/src/content/docs/guides/publishing.md |
| `README.md:107`、`README_ZH.md:107` | docs/PLUGINS.md → website/src/content/docs/writing-a-plugin.md |
| `CONTRIBUTING.md:5` | docs/PLUGINS.md → website/src/content/docs/writing-a-plugin.md |
| `website/src/content/docs/architecture/overview.md:121` | 文件地图里 `docs/{PLUGINS.md,PUBLISHING.md,ACCESSIBILITY.md}` 行 → 改为 `plugins/*/docs/` 与 `website/src/content/docs/` 两个面 |
| `website/src/content/docs/backup-restore.md:84` | 指向 `docs/PRE-LAUNCH.md` 的 GitHub 死链接 → 改为 "maintained outside this repo, `.agy-specs/PRE-LAUNCH.md`" 文字说明 |

**未动的**: `plugins/*/plugin.go` 里的 `docs/<slug>.mdx`(插件自己的帮助目录,非根 docs)与 `plugin/helpdocs_fs.go`/`help_test.go` 的 `docs/` 命名契约 —— 按规格明确不动。

**顺带修正**: `website/.nimbus/routes.json` 构建时自动补上了缺失的 `/backup-restore` 路由(该页面一直存在但路由表漏注册),是构建的稳定产物,随本批次一并提交。

## 5. CI 防复发检查

加在 **`.github/workflows/ci.yml`** 的新 job **`docs-surface`**("Docs surface check",位于 `go` job 之前):

- 不依赖任何工具链,checkout 后 `find docs -name '*.md'` 有命中即失败。
- 错误信息写明两个合法文档面: `plugins/*/docs/*.mdx`(served under `/help`)与 `website/src/content/docs/`。
- 复用已有 workflow 文件,没有新建 workflow。

## 6. 验证结果

| 检查 | 结果 |
|---|---|
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `gofmt -l .` | 无输出 |
| `go test ./plugins/help/ -race -count=1` | PASS (ok, 2.497s) |
| `cd web && pnpm exec tsc --noEmit` | PASS (exit 0,装依赖后) |
| `cd website && pnpm build` | PASS (23 pages, exit 0) |
| `ls docs/*.md` | 无输出 |
| `grep -rn 'docs/[A-Z]'`(go/md/ts/tsx/yml) | 无指向已删文件引用 |
| README gif 路径 | 指向存在的 `assets/octarq-demo.gif` |

# D-driftcode —— 代码注释漂移与冗余

## 结论

对 `.go` / `.ts` / `.tsx` 源码注释做了全量审计：修正 **41 处与代码不符的注释**（含安全声明过强、失效行号/文件引用、幽灵字段、自相矛盾段），删除 **约 149 行冗余/悬空/装饰性注释**，保留全部契约与 "why" 注释。

- 改动范围：66 个文件（58 Go + 8 TS/TSX），+95 / -254 行。
- 全部验证通过：`gofmt -l .` 无输出、`go build ./...`、`go vet ./...`、`go test ./... -race`、`cd web && pnpm exec tsc --noEmit`（0 错误）、`cd web && pnpm exec vitest run`（28 文件 / 101 测试全过）。
- 自查：`git diff` 中非注释代码行变动为 0（脚本见文末）。
- 一个代理曾在其 3 个测试文件中连注释带代码块一起删（List/Update/Export 步骤）——已整体回退为 HEAD，宁保留不越界，故那些文件的注释维持原样。
- `web/node_modules` 在 worktree 中缺失，已用 `pnpm install --frozen-lockfile`（pnpm 9.15.4，未用 npm）补齐后验证；未触碰 `pnpm-lock.yaml`、`webembed/dist`、`cmd/octarq-build/main.go`（生成文件）、任何 markdown。

## 一、与代码不符的注释（每条：file:line + 注释说什么 + 代码实际怎样）

1. **internal/api/recovery.go:319** — "Rate-limited via loginLimiter" — 代码实使用 `h.recoveryLimiter`（recovery.go:329）。改为 "Rate-limited via recoveryLimiter"。
2. **internal/api/register.go:173** — "set for the bootstrap operator account (auth.go:315)" — auth.go:315 是 changePassword 函数体内；`IsInstanceAdmin: true` 实际设在 auth.go:375（bootstrapUserID）。删除错行号，改为 "(see bootstrapUserID in auth.go)"。
3. **internal/api/api.go:65-67** — 声称存在 "emailHandlers are notified … Guarded by emailMu" 字段 — Handler 结构体里没有 emailHandlers/emailMu（已迁往 app/app.go 与 plugins/mail），该字段实际是 `lookupService`。替换为 lookupService 的准确注释。
4. **internal/api/plugins.go:23** — "Features are opt-in: a missing row means disabled" — 代码遇 ErrRecordNotFound 时 `return h.FeatureDefaultEnabled(featureKey)`，未切换的行回落声明默认值。改为 "A row that was never toggled falls back to the feature's declared default"。
5. **internal/api/auth.go:296-300** — "Deliberately not bumping SessionEpoch, which resetPassword does" — `session_epoch` 字段已从代码库消失（grep 仅命中注释）；resetPassword（recovery.go:223-236）只删 session 行从不 bump。重写该段：撤销靠删 session 行+缓存条目，无 epoch 列可 bump，resetPassword 同理。
6. **internal/api/mcp.go:13-14** — "checks the session cookie, the Authorization Bearer header, and falls back to the ?token= query parameter" — mcpAuth 只检查 session org 与 `?token=`，没有 Bearer 头分支。改为 "An authenticated session passes; otherwise a valid ?token= query parameter is accepted"。
7. **internal/api/tokens.go:16** — 引用已删除的 `tokenAlphabet` 常量、称"length-independent random token body" — 实现是 base64.RawURLEncoding。删除失效首句，保留 "why" 句。
8. **internal/api/api.go:198** — "// Auth (no session required)." — 该段同时注册需 session 的 me/password/email/sessions 等路由。改为中性的 "// Auth routes."。
9. **internal/api/public_endpoints.go:30** — "(auth.go:189)" — auth.go:189 是 verify2FA 体内，非 logoutAll；logoutAll 的 AuthenticateRequest 在 auth.go:245。改为 "(see logoutAll)"。
10. **internal/api/helpers.go:22** — "returns the best-guess client IP for abuse reports" — reporterIP 广泛用于 login/recovery/register limiter、audit、abuse。首句改为 "for rate limiting and abuse reports"，保留第二句 "why"。
11. **internal/api/recovery_test.go:333** — "Re-trigger forgot password to get valid token" — 代码直接调用 `generateSecureToken()` 写库，未走 forgot 流程。改为 "Seed a known, unexpired token directly (bypassing the mail path)."。
12. **internal/api/coverage_test.go:171-172** — 悬空注释 "TestInboundWebhookAuth covers…" — 该测试函数全库不存在（已删除/改名）。整段删除。
13. **internal/api/invite_dns_test.go:214-217** — 悬空注释 "TestVerifyDNSMailHosts … / TestVerifyDNSLinkHosts …" — 这两个测试已迁至 plugins/dns/verify_test.go，本文件无对应函数。整段删除。
14. **internal/api/actions_test.go:69** — 步骤编号 "4." 而上一编号是 "2."（无 3）。改为 "3."。
15. **internal/api/abuse_test.go:40** — 步骤序列 1、3、4 缺 2（rate-limit 块无编号）。补上 "2. " 前缀。
16. **internal/api/public_endpoints_test.go:38-42** — "rate-limited as its own tier in internal/server/middleware.go:205" — `POST /abuse` 走 auth 档位（middleware.go:202 `return tierAuth`），真正自有限流是 handler 的 `abuseLimiter`（5/hour，abuse.go:55/99）；middleware.go:205 也不是限流点。改为 "handler rate-limits it with its own 5/hour abuseLimiter…and the middleware additionally gives POST /abuse the auth-tier budget"。（数据 map 值保留原样，只改注释）
17. **internal/api/public_endpoints_test.go:129** — 行引用 "auth.go:189" — logoutAll 定义于 auth.go:240，重新认证在 auth.go:245。改为 `auth.go:245`。
18. **internal/api/register_test.go:153** — 行引用 "auth.go:72" — 登录拒绝未验证用户的实际检查在 auth.go:74。改为 `auth.go:74`。
19. **internal/api/link_redirect_validation_test.go:38** — "Slug 1 (first link)" — 测试未指定 slug，指向的其实是第一条链接的 id 1。改为 "The first created link gets id 1 — update by id 1."。
20. **app/app.go:772-773** — "can this instance send mail is a lookup of the **mail.send** service" — 代码查询 `plugin.ServiceMailReady`（app.go:784），且紧邻注释（app.go:778）明确说 "no longer answered by the mail.send service"。`mail.send` → `mail.ready`，消除自相矛盾。
21. **app/app.go:321-325** — sendMail 注释称自己 "resolves the org's first configured SMTP sender, decrypts its password, and relays the message" — 代码仅委托 `plugin.LookupServiceAs[plugin.MailSender](…, plugin.ServiceMailSend)`（app.go:326-330）。改述为委托关系。
22. **plugin/plugin.go:404** — "ActivePlugins returns a **snapshot** of all registered plugins" — 代码直接返回 `a.plugins` 活切片（app.go:691-692），非拷贝。改为 "returns the currently registered plugins"。
23. **internal/dnsprovider/provider.go:3** — "Cloudflare is implemented today… keep room for Aliyun / DNSPod / Route53" — `dnspod.go` 已完整实现并 `Register("dnspod", …)`。改为 "Cloudflare and DNSPod are implemented today…"。
24. **internal/auth/auth.go:170-171** — 常数时间比较使 "neither the username nor password leaks length/prefix information via timing" — `subtle.ConstantTimeCompare` 长度不等时立即返回，长度可观测，安全声明过强。重写：比较无定时侧信道、不泄露前缀信息，但仍暴露长度。
25. **internal/server/server.go:111** — "(gated to the admin host when configured)" — 引用已删除的 `OCTARQ_ADMIN_HOST`；现由 `dashboardAllowed`（origin.ServesTraffic）把关（同文件 232-236 行已注明该变量废弃）。改为 "(gated by dashboardAllowed)"。
26. **internal/models/models.go:20-22** — `SingleUserID` 注释称 "handlers will replace it" — 全仓已无任何 handler 引用该常量。改写为 legacy 占位常量、仅供向后兼容。
27. **internal/models/models.go:51** — User 结构体中间悬空 "Session revocation is handled by deleting Session rows from user_sessions." — 与相邻字段无关且别处已有表述。删除。
28. **internal/models/slug.go:37** — 交叉引用 "(api.isReservedSlug)" — 该符号在 internal/api 已不存在（短链保留字表迁入 plugins/links）。修正为 "the links plugin's isReservedSlug"。
29. **internal/mcp/mcp.go:56-79** — doc 块挂在 `NewNetworkedServerInstance` 名下，开头却是 "RunWithPlugins is identical to Run… allowRawSQL gates…"，且声称 "It also never Mounts the plugins" — 一、`RunWithPlugins` 无 allowRawSQL 参数且走 `NewServerInstance`→`buildServerInstance(..., mountPlugins=true)` 会 Mount；二、`NewServerInstance` 无 doc。将 NewNetworked 专属段归其名下、allowRawSQL 契约写成 NewServerInstance 的 doc（77-81 行）、开头两行重新安给 RunWithPlugins 并补 stdio 语义（129-134 行）。
30. **internal/mcp/mcp.go:160-162** — "see NewServerInstance for why it is withheld" — 理由已不在 NewServerInstance 上。改为指向 NewNetworkedServerInstance。
31. **internal/mcp/remount_guard_test.go:13** — 行引用 "plugins/links/plugin.go:94-96, plugins/mail/plugin.go:105" 指向无关代码（docs embed、Actions 菜单）— 实际缓存 `ctx.OrgID` 的 Mount 块在 links:120-122、mail:127-129。改为正确行号。
32. **origin/origin.go:182** — "this needs five columns" — `domainRow` 读 6 个 gorm 列（owner_id, name, for_link, for_mail, link_hosts, mail_hosts）。改为 "six columns"。
33. **origin/origin_test.go:354-357** — "TestResolverCaches pins…" 段落挂在 TestResolverOwnerOfCaches 名下；TestResolverCaches（现 393 行）无 doc。两段各归其主。
34. **main.go:94-97** — 注释称 hello 插件 "has no Describer… off by default, opt-in from Settings → Plugins" — 代码示例实现 `plugin.Describer` 且 `EnabledByDefault: true`。重写为真实行为。
35. **plugins/links/shortlink.go:1** — 包文档 "Package shortlink" — 文件实际 `package links`（改名残留）。改为 "Package links"。
36. **web/src/shell/areas.tsx:77** — "Links → core plugin (plugins/core/links.ts…)" — links 前端位于 plugins/links/（manifest 组合的 feature plugin），plugins/core/links.ts 不存在；category "Marketing" 正确（plugins/links/plugin.go:83）。改为 "// Links → plugin (plugins/links, category "Marketing")."。
37. **web/src/shell/areas.tsx:79** — "Mail → core plugin (plugins/core/mail.ts…)" — 实际 plugins/mail/，plugins/core/mail.ts 不存在；category "Messaging" 正确。改为 "// Mail → plugin (plugins/mail, category "Messaging")."。
38. **web/src/shell/areas.tsx:96-97** — "DNS → core plugin (plugins/core/domains.ts)" — 实际 plugins/dns/，plugins/core/domains.ts 不存在；category "Network" 正确。改为 "// DNS → plugin (plugins/dns); …"。
39. **web/src/shell/menuStyles.ts:1-4** — "AreaPanel still holds its copy and is migrated in the batch…" — AreaPanel.tsx:19 已 import MENU_ITEM/MENU_POPUP from "./menuStyles"，迁移已完成。改为 "both consume this module now"。
40. **web/src/api.ts:646-651** — "/api/auth/methods has exactly one consumer — plugin-sso's login button" — security.tsx:213（SecuritySettings SSO 检测）也 fetch 消费，共两个消费者且都直接调 fetch()。更新为两消费者描述。
41. **web/vite.config.ts:64-66** — "Remove the /instance branch with the missing backend route (see report)" — internal/server/server.go:149 明确服务 `/instance` 路由，不存在 "missing backend route"。改为 "All three branches mirror what server.go already serves"。

<!-- APPEND_REMAINDER -->

## 二、删除的冗余注释（统计 + 典型例子）

**统计**：删除约 149 行。按类目：

- **装饰性分节分隔线 / 分隔注释**（约 40 行）：`internal/server/middleware.go` 12 条纯 `// ----…` 分栏线（保留各自信息标题）；`internal/queue/queue.go:49/126` 的 `// --- InMemoryQueue implementation ---`（复述下一行类型名）；`internal/mcp/mcp.go:183`、`internal/mcp/tools.go:15/48`；`web/src/ui/charts.tsx:80`、`web/src/ui/HostList.tsx:122` 文件末尾悬空的 `// ─── timeAgo ───` / `// ─── Guide ───`（组件实际在 ui/time.ts 与 ui/primitives.tsx）。
- **复述代码/函数名的编号步骤与标签**（约 90 行）：`internal/api/comprehensive_api_test.go` 32 条 CRUD 步骤标签（"// Create SMTPSender" 等，下一行即同义 HTTP 调用）；`role_baseline_test.go` setup 复述；`audit_abuse_gate_test.go` 3 条；`recovery_test.go:265/428` "// Register a user"；`status_test.go:85` "// Verify no sensitive fields…"；`ratelimit_test.go:9` "// Limit: 2 failures per 100ms"（复述构造参数）；`cmd_backup.go:74/99` 与 `cmd_backup_test.go` 6 处编号步骤；`openapi/openapi.go` 8 处 "// --- X Endpoints ---"；`cmd/openapi-gen/main.go:18`；`plugins/links/links.go:791` 悬空类型文档；`dns/ddns_test.go`、`dns/ddns_extra_test.go`、各 plugin_meta_test.go 的步骤标记；`plugins/mail/mail.go:48` "// Attach unread counts."。
- **与上方块注释重复的行尾/毗邻注释**（约 8 行）：`llmprovider/provider.go:38-39` 常量行尾 `// complex reasoning` / `// cheap classification / summary` 与上方 8 行块注释重复；`plugins/mail/mail.go:637` 重复的路由标记（webhook.go:441 已有同款）。

**典型例子**：
- `plugins/links/links.go:791` — 悬空注释 "// models.StatKV is a key/count pair…"（为导入类型写的文档，位于无关的 LinkQRInput 上方，与 internal/models 该类型文档重复）。
- `internal/api/coverage_test.go:171-172` — "TestInboundWebhookAuth covers…"（测试已不存在，见第一节）。
- `web/src/plugins/dns/pages/index.tsx:426`、`web/src/plugins/mail/pages/Compose.tsx:118-119` — 文件末尾悬空注释（badge 组件实际在 dnsStatus.tsx；AuthBadges 在 EmailView.tsx）。

保留的删/改二义性注释（未纳入删除统计）：如 "// Delete Domain - member forbidden / admin success"（区分同一操作的两个变体）、编号步骤中承载预期状态码的行为导航注释。

## 三、犹豫但保留的清单（file:line + 为什么犹豫）—— 给人复核用

1. **plugin/plugin.go:240-242** — "// UserID extracts…(0 if unauthed)." 错放在 `RevokeUserOrgSessions` 字段上方，`UserID` 字段反无注释。修正需把注释移入 gofmt 对齐的字段区间（会改动代码行空白，且任务禁止移动行）；删除会丢失契约说明 → 保留。
2. **app/app.go:748-750** — "Launch Starters only after EVERY plugin has mounted…" 描述的是 761-765 行的 Starter 循环，位置却在 751-759 的 deferred-OnEmail flush 块上方（重构插入所致）。文本内容真实、不矛盾，移动被禁止 → 保留。
3. **plugin/sqlguard.go:29** — BannedKeywords 注释 "statement types" 措辞略宽（含 pragma/attach 等关键字），但仍准确描述，非矛盾 → 保留。
4. **plugin/perm.go:28-34** — DeclarePerm 的 "Keyed, not appended… Mount runs more than once"（防重复列举的 why）→ 保留。
5. **app/app.go:277-278** — 引用 entry point "octarq/main.go"；仓库根确有 main.go（无 octarq/ 目录），可读作 "octarq 仓库的 main.go" → 保留。
6. **internal/queue/queue.go:112,147** — "Queue full, fallback to instant goroutine execution" / "Connection failed - run instantly in background as fallback"：接近复述，但对 fallback 路径有上下文价值 → 保留。
7. **internal/server/middleware.go 各标题行** — "// Metrics (stdlib expvar)" 等 6 行：只删了纯装饰分隔线，标题保留（"ResponseWriter wrapper" 还是 statusRecorder 的唯一文档）。
8. **internal/eventbus/eventbus.go:137** — "Calculate HMAC-SHA256 signature over the plaintext signing secret."：略重复 deliver 文档，但点出"明文签名密钥"非显然事实 → 保留。
9. **internal/server/middleware.go:344-346** — settingsRefreshInterval 注释列举 "rate limits, metrics token"，实际 refreshConfig 也刷新 CORSOrigins：不完整但并不错误（示例式列举），且属契约性说明 → 保留。
10. **internal/auth / internal/tenancy / internal/db / internal/server 中所有含 MUST/NEVER/隔离/一次检查 语义的注释** — 属规则保护范围，全部未动。
11. **internal/mcp/mcp.go:164-169** — registerTools 函数 doc（159-162）与体内内联注释实质重复；因是 raw-SQL 安全不变式、两处各自陈述似有意 → 保留。
12. **internal/mcp/audit.go:21 (:52)** — "ActorID // 0 = system / AI" 在字段、实例化、包 doc 三处重复；均属小语境 → 保守保留。
13. **origin/origin.go:24-26 与 octarq-pro pkg/baseurl 的对照** — 无法在只读的 pro 仓库外验证 → 未动。
14. **origin/origin.go:119-123 与 zoneCandidates 自身 doc:143-145** 略有重叠，但各自补充安全理由/截断语义 → 未删。
15. **origin/origin.go:610/629/648** — "X is the cached form of the package-level X" 属成对说明模式，非冗余。
16. **internal/api/token_scope_test.go:15-29** — 包级注释描述 token 授权模型历史沿革，涉安全语义 → 保留。
17. **internal/api/register_test.go:82-86** — "exactly as before the field existed" 解释行为基线 → 保留。
18. **internal/api/instance_readiness_test.go:161-164** — "the old field is removed in a later batch" 为前瞻性说明；settings.go:300/552 仍输出 `isInstanceAdmin`，暂未过期 → 保留。
19. **internal/api/recovery_test.go:1-27** — failUserWrites 的 GORM 机制说明，属陷阱/契约注释 → 保留。
20. **internal/api 各测试文件的步骤编号注释**（"// 1. Register user and log in…"）— 承载预期状态码，视作行为导航而非纯复述 → 保留。
21. **plugins/help/help_test.go:124-125** — 两条相邻注释解释 slug 冲突消解，前半是预留条件后半是现有顺序下的确定结果，共同承载"顺序决定谁改名"的测试前提 → 保留。
22. **cmd_backup.go:87-88** — 改写后的 safety-net 注释，属设计意图（破坏性恢复前必须自备份）→ 保留。
23. **各 models.go 字段注释**（"AES-GCM encrypted credentials JSON"、"empty = default/any host"、"pass|fail|softfail|neutral|none"）— 编码/语义无法从类型推断 → 保留。
24. **plugins/dns/records_verify.go:37,51,65** — SPF/DMARC/DKIM 步骤注释解释各记录 DNS 位置（`_dmarc.<host>`、`<selector>._domainkey.<host>`），属非显然领域知识 → 保留。
25. **plugins/mail/mail.go:661,708,747** — bounce 解析 "1. SES / 2. Mailgun / 3. SendGrid" 分支标记，标识不同供应商载荷格式 → 保留。
26. **main.go:91-97 重写版及 config/coreschema/llmprovider 中所有 约定/MUST/安全 契约注释** — 逐条核对与代码一致，全部保留。
27. **plugins/dns/crud_handlers_test.go、plugins/links/crud_handlers_test.go、plugins/mail/crud_handlers_test.go** — 原始裁决是删除其中的编号步骤注释；但代理在此三文件里连代码块一起删（List/Update/Export），为守住"零代码行变动"底线已整体回退为 HEAD，注释一并保留。如需清理可后续单独按注释-only 重做。
28. **web/src/shell/areas.tsx:83,85** — "Abuse Reports → core plugin (plugins/core/abuse.ts…)" 与 "Audit Log → core plugin (plugins/core/audit.ts…)"：这两个文件确实存在于 plugins/core/，category 与 tenant_menu.go:719-720 一致，是正确的 → 保留。
29. **web/src/shell/areas.tsx:80-81,99-100** — "@octarq-org/plugin-ai" / "@octarq-org/plugin-infra" 是 Pro 插件引用，代码库内无法核验存在性，属契约性描述 → 保留。
30. **web/src/api.ts:646-651 其余部分** — "typed wrapper had no reachable caller"、"Expose through @octarq/plugin-sdk" 等设计决策说明 → 保留。
31. **packages/plugin-sdk/** 各文件 JSDoc** — 已逐条核对与实现一致（registry.ts replace 语义、types.ts UIPlugin 契约），是公开 API 契约 → 全部保留。
32. **internal/api/status_test.go、coverage_test.go、csrf.go、host_org.go、org_slug.go 等 long-form 设计注释** — 逐条核对过代码一致 → 保留。

## 自查命令与结果

```bash
# 非注释代码行变动检查（最强版）：剥离注释语法后，逐文件比对 DEL 行集合与 ADD 行集合。
# 要求每一条剥离后的删除行在剥离后的新增行里有逐字节相同的配对 —— 代码 token 零变动。
python3 /tmp/drift_diff_check3.py
# 结果：PROVEN — every stripped deleted line has an identical stripped added line per file
#       → 66 个文件的所有编辑均为注释-only（含 llmprovider/provider.go 的行尾注释删除，
#         剥离后两侧代码部分完全相同）。

unset http_proxy
gofmt -l .              # 无输出
go build ./...          # 通过
go vet ./...            # 通过
go test ./... -race     # 全部 ok
cd web && pnpm exec tsc --noEmit   # 0 错误（先 pnpm install --frozen-lockfile 补齐 node_modules）
cd web && pnpm exec vitest run     # 28 files / 101 tests passed
```
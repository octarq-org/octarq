# FIX-ux 报告 —— 前端 UX 一致性修复

分支 `fix/ship-ux`，工作区 `.worktrees/octarq-ship-ux`，只动了 `web/**`。
依据：`AUDIT-ux.md`（.worktrees/octarq-ux，只读）。未改任何 Go 文件、未动
`webembed/dist`（本次验证期间的构建产物已还原，未提交）。

## 1. 插件路由懒加载白屏

`web/src/plugins/PluginRoutes.tsx:97` 的 `<Suspense fallback={null}>` 改为
`<Suspense fallback={<RouteFallback />}>`，与 core 静态页（Settings.tsx）一致。

实现细节：`RouteFallback` 从 `web/src/App.tsx` 移到了
`web/src/components/ui/RouteFallback.tsx`，`web/src/ui.tsx` barrel 导出，App.tsx
re-export 保持既有 `import { RouteFallback } from "../App"`（Settings.tsx）兼容。
移动而非直接从 App 导入是为了避免 `App → PluginRoutes → App` 模块环。
`PluginRoutes.tsx` 加了防回退注释（fallback={null} 会遮蔽外层 Suspense）。

验证：CDP 1.5s 延迟下 SPA 导航到 `/admin/domains`，chunk 加载期间 DOM 存在
`[role="status"] .animate-spin`（RouteFallback spinner）；修复前此处是白屏。
截图 `../../.agy-specs/uxfix-shots/spinner-domains.png`（light，
`html.class=""`）。

## 2. 列表页加载态（共用骨架组件）

新增共用组件 `web/src/components/ListSkeleton.tsx`（rows/ariaLabel 可配：
GlassCard + N 行「标题/副行」骨架块，`aria-busy` + `role="status"`）。

**放 `web/src/components/` 的理由**：五个使用方（links/mail/dns 三个插件页 +
core 的 Abuse/Audit）都在本仓库内，`web/src/components/` 是 core 页面
（`web/src/pages/`、`web/src/plugins/`）都能直接 import 的公共层；
`packages/plugin-sdk/src/ui` 是跨仓库发布的 SDK 原语，骨架屏不是插件契约能力，
进 SDK 需要动包版本且让 Pro 侧也能用上，超出本次「一致性修复」的范围。

五个页面统一接入（都是 `items.length === 0 && loading` 的初始加载分支，不再落
到 else 分支的底部一行 "loading"）：
- `web/src/plugins/links/pages/index.tsx`、`web/src/plugins/mail/pages/index.tsx`、
  `web/src/plugins/dns/pages/index.tsx`：列表卡内首屏渲染 `<ListSkeleton rows={7} />`
  （原 `length===0 && !loading` 空态分支相应改判 `length === 0`）；底部
  load-more 的 "loading" 行保留（那是分页态，不是首屏态）。
- `web/src/pages/Abuse.tsx`（rows=5）、`web/src/pages/Audit.tsx`（rows=8）：
  `loading` 分支从一行文字换成 `<ListSkeleton />`。

每处传 `ariaLabel={t("<ns>.loading")}`，五个 loading key 全部保持被引用
（i18n audit 无孤儿告警）。

验证：CDP 1.5s 延迟下首次进入 `/admin/links`，列表区 DOM 有 `aria-busy=true` +
14 个 `.animate-pulse` 骨架块（7 行 × 2），截图像素为灰块行，非空框、无
"loading…" 文字。截图 `../../.agy-specs/uxfix-shots/skeleton-links.png`（light，
`html.class=""`）。

## 3. 死 i18n key

删除前 grep 复核（octarq 侧 `web/src` + `plugins`，Pro 侧
`/Volumes/PHD/code/octarq-pro` 只读，ts/tsx/go 全覆盖）：
- `nav.certs` / `nav.databases` / `nav.storage`：**两仓均无消费点**。Pro 的
  证书/数据库/存储菜单 id 是 `certificates`/`databases`/`storage`，翻译由 Pro
  插件经 `_shared.nav.*` 提供（如
  `octarq-pro/packages/plugin-infra/src/i18n.ts:6` 的
  `_shared.nav.certificates`、`_shared.nav.storage`），与 core 的死 key 不同名；
  Pro 侧没有任何引用 core 词典 `nav.certs/databases/storage` 的代码。
- `groups.Subscriptions`：**两仓均无消费点**（无任何菜单使用
  `Category: "Subscriptions"`）。

删除清单（五个词典同步删）：`nav.certs`、`nav.databases`、`nav.storage`、
`groups.Subscriptions`，文件：`web/src/i18n/{en,zh,es,pt,ja}.ts`。
顺带验证：core Go 菜单 id（`internal/api/tenant_menu.go:719-722` 等）只有
overview/audit/abuse + 设置项，无 `certs/databases/storage`，删除安全。

## 4. 设置侧栏标签

复核结论（与审计 1.5 一致）：**侧栏设置项已经全部走词典** —— `AreaPanel.tsx`
对所有条目调 `translateNavItemLabel(t, item.id, item.label)`（`:165-166,213,241`），
`SETTINGS_AREA` 全部 11 个 id（general/plugins/members/webhooks/notifications/
profile/security/tokens/auth/instance/instance-plugins）在五个词典里都有
`nav.<id>` key，id 均为连字符安全格式（无点号）。`areas.tsx:113-140` 的
"Features"/"Members"/"Instance Settings" 只是永不渲染的 fallback。

本次落地：
- `web/src/shell/areas.tsx:140`：过期 fallback `label: "Plugins"` →
  `"Installed plugins"`（对齐 `nav["instance-plugins"]` 与页面标题
  `instancePluginsTitle`）。
- `web/src/shell/areas.tsx:86-103`：清理指向已删除 `plugins/core/assets.ts`
  的过期注释（Certificates/Storage & Databases 已由 Pro 插件提供）；分组壳
  （Hosting、Storage & Databases）保留 —— 它们由 Pro 菜单按 category 填充，
  删掉会丢菜单。
- 设置页正文标题本就走 `t()`（`web/src/i18n/pages/settings.ts`），无需改。

验证：zh 界面截图（localStorage `lang=zh` 预热）侧栏显示
通用/功能/成员/Webhook/告警 · 我的资料/安全/API令牌 · 身份验证/实例配置/已安装插件，
与正文同语言。截图 `../../.agy-specs/uxfix-shots/settings-general-zh.png`（light，
`html.class=""`，lang=zh）。

## 5. Overview 引导清单 2FA 步

选择：**状态未知时隐藏该步**（不是「不判定为未完成」，也不是标成未完成）。

理由：`SetupStep` 只有 `completed: boolean`，没有第三态；接口失败（或请求在飞）
时 `twoFAEnabled === null`。若标成未完成，已开 2FA 的用户永远看到该步未完成、
进度条永远不满（原 bug）；若新增「未知」视觉态，需要动 SetupStep 的样式与
无障碍，为一个罕见失败场景加复杂度。隐藏该步让清单进度只统计已知状态的步骤：
状态请求在飞时短暂隐藏（数百 ms，无感），失败后不再出现；若 2FA 是清单里唯一
一步且状态未知，整卡按 `totalCount === 0` 规则自然隐藏，不会给出错误进度。

实现：`web/src/pages/Overview.tsx` 的 steps 数组在 `twoFAEnabled === null` 时
不包含 2fa 步（`completed: twoFAEnabled === true` 判定保留）。

验证：Overview 正常渲染（2FA 状态已知，步骤显示 "Secure Your Account"）。
截图 `../../.agy-specs/uxfix-shots/admin-overview-light.png` / `-dark.png`
（`html.class=""` / `"dark"`）。

## 6. 文案同义异名统一

只改文案不改 key。逐组最终选词：

| 组 | 选词 | 改动文件 |
|---|---|---|
| 破坏性删除 | **Delete / 删除 / Eliminar / Excluir / 削除**（删除域名、删除主机）；非破坏性保留 **Remove / 移除 / Quitar / Remover / 解除**（从抑制列表移除） | `web/src/plugins/dns/i18n.ts`（en `removeConfirm`→`Delete {{name}}?`、`removeHost`→`Delete host`、`remove`→`Delete`；zh 移除→删除 ×3；pt `Remover`→`Excluir` ×3；es/ja 原本已是 Eliminar/削除）；`web/src/plugins/mail/i18n.ts`（抑制列表为非破坏移除，en/zh 保留 Remove/移除；es `Eliminar`→`Quitar`、ja `削除`→`解除`，对齐 en/zh 的非破坏语义） |
| 短链/短链接 | **短链** | `web/src/plugins/links/i18n.ts` zh（pageTitle `短链接`→`短链`、pageDescription、tabLinks、pluginDesc 四处）；`web/src/i18n/pages/settings.ts` zh（`短链接设置`→`短链设置`、`保留的短链接别名`→`保留的短链别名`）；`web/src/i18n/pages/overview.ts` zh（stepDomainDesc、stepLinkTitle、stepLinkDesc、shortLinks 四处）。`grep 短链接 web/src` 已零命中 |
| 启用/开启 | **启用**（动词语境） | `web/src/plugins/mail/i18n.ts` zh `guideStep1Mid`「开启」→「启用」；`web/src/i18n/pages/overview.ts` zh `step2FADesc`「开启双重验证」→「启用双重验证」。描述性 prose 的「开启/关闭」对（settings 页说明文字）保留不动，理由：那是成对惯用语而非动作动词，audit 清单也只点了 mail 引导一处 |
| Audit Log/Logs | **Audit Log** | 复核：本仓 `web/src` 内无 "Audit Logs" 残留（`nav.audit`=“Audit Log”、`audit.pageTitle`=“Audit Log” 已统一）。Pro `plugin-audit` 的 "Audit Logs"（`octarq-pro/packages/plugin-audit/src/i18n.ts:5`）属审计 1.3 判定的死代码，且本线不改 octarq-pro，不动 |
| DNS vs Domain/域名 | 维持 **DNS** | 审计判定为可接受的已知命名，复核同意：菜单/页标题/记录管理语境统一 "DNS"，未发现审计判断错误 |

未纳入本次范围的：settings 页共享 `remove`（成员移除/提供商/发件配置移除等非
数据删除语境，审计清单未点名）、Pro 侧 storefront/whitelabel 的文案漂移
（octarq-pro 只读）。已在报告中说明，留待 Pro 侧线处理。

## 验证结果（`unset http_proxy && cd web`）

- `pnpm exec tsc --noEmit` ✅ exit 0
- `pnpm test` ✅ 与基线同 profile：唯一不稳定的是 `brandRefresh.test.tsx`（5s 超时）。
  做了受控 A/B 验证排除本次改动的因果性（本机负载极高，collect 阶段 143-340s）：
  - 基线（.worktrees/octarq-ux，audit/ux 分支，改动前代码）全量 3 跑：
    `App.test.tsx` 3/3 绿；`brandRefresh` 2/3 跑超时
  - 本分支全量 5 跑：最早 2 跑处于全机峰值负载期，`App.test.tsx` 超时
    （失败细节为 2s waitFor 在 shell 已渲染、内容区仍在挂载时到期，纯时序）；
    当前负载下 2/2 全绿（仅 brandRefresh 超时），与基线 profile 一致
  - `App.test.tsx` 单跑/组合跑共 9/9 绿；`brandRefresh` 两分支同率超时
  - 结论：`brandRefresh` 是既有机器负载 flake（本线未触碰该文件及 brand 代码），
    `App.test` 的间歇失败只出现在峰值负载窗口，本改动对其执行路径完全惰性
    （RouteFallback 在测试中不渲染、无新增模块环、tsc 通过）
- `pnpm i18n:audit` ✅ "All i18n checks passed!"（1117 static keys resolve，
  5 词典平权、11 个 Go 菜单 id、8 个图标 key 全绿；无新增 orphan 告警）
- `pnpm build` ✅ exit 0（构建产物落 `webembed/dist`，已还原未提交）

## 截图证据

拍摄方式：先 `pnpm build` 再重启后端（`go run .` 内嵌新 dist，顺序未反），
`shot.mjs` + 一个临时补充脚本（CDP 限速抓瞬态加载态；已删除，未提交）。
所有截图均打印 `document.documentElement.className`（`""`=light，`"dark"`=dark）。

| 文件（仓库相对路径） | 证明内容 | 主题 |
|---|---|---|
| `../../.agy-specs/uxfix-shots/admin-links-light.png` / `-dark.png` | 插件懒加载路由完整渲染（标题 Links + 空态/列表），无白屏 | `""` / `"dark"` |
| `../../.agy-specs/uxfix-shots/admin-mail-light.png` | Mail 页完整渲染 | `""` |
| `../../.agy-specs/uxfix-shots/admin-domains-light.png` | DNS 页完整渲染 + 引导卡 | `""` |
| `../../.agy-specs/uxfix-shots/admin-abuse-light.png` | Abuse 页完整渲染 + 空态 | `""` |
| `../../.agy-specs/uxfix-shots/admin-audit-light.png` | Audit 表页完整渲染 | `""` |
| `../../.agy-specs/uxfix-shots/skeleton-links.png` | 首屏 ListSkeleton（DOM：aria-busy + 14 个 pulse 块） | `""` |
| `../../.agy-specs/uxfix-shots/spinner-domains.png` | chunk 加载期间 RouteFallback spinner（DOM：role=status + animate-spin） | `""` |
| `../../.agy-specs/uxfix-shots/settings-general-zh.png` | zh 侧栏全本地化（功能/成员/告警/实例配置/已安装插件…） | `""` lang=zh |
| `../../.agy-specs/uxfix-shots/links-zh.png` | zh 页标题统一为「短链」 | `""` lang=zh |
| `../../.agy-specs/uxfix-shots/admin-overview-light.png` / `-dark.png` | Overview 清单含 2FA 步、进度条正常 | `""` / `"dark"` |
| `../../.agy-specs/uxfix-shots/admin-settings-general-light.png` | 侧栏英文标签逐项渲染（General…Installed plugins） | `""` |

（截图存于 `/Volumes/PHD/code/.agy-specs/uxfix-shots/`，工作区外共享目录，不入库。）

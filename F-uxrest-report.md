# F-uxrest — UX 审计剩余项报告

分支：`fix/uxrest`（worktree `/Volumes/PHD/code/.worktrees/octarq-uxrest`）
依据：`.agy-specs/AUDIT-2026-08-16/AUDIT-ux.md` Top 10 中的 #4/#5/#9/#10 + 两项顺手清理
范围：只动 `web/` 与 `packages/plugin-sdk/`，未改任何 Go 文件。

---

## #5 三份手写空态收敛到共享 `Empty`（主任务）

**现状确认**：前一轮只给 links/dns 页面导入了 `Empty` 却未使用（空态仍是手写 div 副本），mail 页完全未导入。三份空态结构（`flex flex-col items-center gap-2 px-4 py-8 text-center` + reason + detail + action）几乎逐字重复。

**改法**：三处列表空态全部改为渲染共享 `packages/plugin-sdk/src/ui/primitives/empty.tsx`（经 `web/src/ui.tsx` → `web/src/ui/primitives.tsx` 桶导出），删掉手写副本：

- `web/src/plugins/links/pages/index.tsx:197` — 无链接空态改用 `<Empty reason/detail/action>`，icon 槽用 `Link2`；筛选无结果分支（`emptyFilteredReason`）保留原样（那是查询态，非"无数据"态）。
- `web/src/plugins/mail/pages/index.tsx:230,238` — 无邮箱空态与空收件箱空态均改用 `<Empty>`（icon 槽 `MailIcon`/`Inbox`）。
- `web/src/plugins/dns/pages/index.tsx:360` — 右栏引导卡改为 `<Empty>`（icon 槽 `Globe`，reason/detail/action 三槽承载原引导卡全部内容，provider 感知逻辑原样保留）。

## #4 DNS 无数据同屏两套空态

**改法**：`web/src/plugins/dns/pages/index.tsx:195` 左栏纯文本残留分支（`domains.emptyNoDomainsReason`）删除，改为 `null`（有意为空，注释说明空态在右栏引导卡，防止回归）。右栏引导卡（现为共享 `Empty`）成为唯一空态。

**顺手清理**：`emptyNoDomainsReason` key 在五份 dns 词典中已无消费点，从 `web/src/plugins/dns/i18n.ts` 全部删除（5 处）。

## #9 LockedFeature 升级按钮指向已废路径

**真实路径查证过程**：
1. `packages/plugin-sdk/src/ui/locked.tsx:89` 硬编码 `window.location.href = "/admin/license"`。
2. 查 `web/src/App.tsx`：`/license` 路由重定向到 `/settings/license`（`App.tsx:645`），即 `/admin/license` 靠"basename `/admin` + `/license` 重定向"两层兜底才命中 license 页 —— 审计 #3.4 指出的脆弱耦合。
3. 查 Pro 侧（octarq-pro 只读）：`octarq-pro/packages/plugin-licensing/src/index.ts:11` 注册路由 `path: "/settings/license"`；`octarq-pro/modules/licensing/licensing.go:86` 的 `Menus()` 返回 `Path: "/settings/license"`（Category "Instance"，落 Settings → Instance 组）。
4. git 历史确认：`ccc7bf1 refactor: move the Settings-area pages into /settings/*` 把 license 路由从 `/license` 迁到 `/settings/license`，此后 `/settings/license` 是 license 页的真实路由。
5. 实例运维台（`/instance`，commit d7c1c86）落地后 license 归属**未再变动**：`/instance` 只接管 auth/settings/plugins 三页（`web/src/pages/instance/` 无 license 页），license 仍在 Settings 区。

**结论与改法**：真实路径 = `/settings/license`（router 相对）；`window.location.href` 是全页面跳转（不走 BrowserRouter basename），故绝对 URL 必须带 `/admin` 前缀 → 改为 `window.location.href = "/admin/settings/license"`（`locked.tsx:89`），直接命中 license 页，不再依赖 `/license → /settings/license` 重定向兜底。

**changeset**：`.changeset/locked-feature-license-link.md`，级别 **patch**。
理由：这是对已发布 npm 包（`@octarq/plugin-sdk` 0.10.0）的 bug 修复（修正错误导航目标，消除脆弱依赖），不新增 API、不改变组件签名、不破坏兼容 —— 遵循仓库历史惯例（`fix(plugin-sdk): surface UIPlugin name collisions` 等修复均用 patch）。CI 门禁要求改了 SDK 源码必须带 changeset，否则 `publish-sdk.yml` 永远发不出去。

## #10 后端错误串未本地化

**改法**：
- `web/src/components/ui/FormError.tsx` 新增映射层：
  - `formErrorStatusKeys`：401/403/404/429/500 → `uiCommon.errStatus*` 五语言文案。
  - `formErrorMessage(err, t)`：命中映射用本地化文案；**未命中（未知 status 或直接字符串）原样回落后端 `error`/`detail` 原文**，不吞成笼统"发生了错误"。
  - `FormError` 组件改用 `formErrorMessage` 取 message，status/requestId mono 行保留。
- `web/src/plugins/mail/pages/MailSettings.tsx` 补失败反馈：`save()` 增加 catch，把 `ApiError` 的 message/status/requestId 存入 `err` state，保存按钮下方渲染 `<FormError>`（此前保存失败完全静默）。
- 新增 5 个 key 进 `web/src/i18n/pages/uiCommon.ts` **全部五个词典**（en/zh/es/pt/ja）：`errStatus401/403/404/429/500`。

## 顺手清理（低危）

先 grep 核查了 `web/src/shell/areas.tsx` 的两处目标，**均已在前几轮修复，无需改动**：

- `areas.tsx:86-103` 过期注释：引用已删除的 `plugins/core/assets.ts` 的注释（"Certificates → core plugin (plugins/core/assets.ts)"、"Databases + Object Storage → core plugin"）已在 commit `8019c4e fix(web): close UX consistency gaps…` 更新为 Pro 插件说明；当前文件 96-104 行已无 `plugins/core/assets.ts` 引用（grep 0 命中）。
- `areas.tsx:140` 过期 fallback 标签 "Plugins"：实例运维台 commit `d7c1c86` 已把 Instance 组改为 `/instance` 入口（label "Instance Management"，external），"Plugins" 静态标签不存在（grep 0 命中）。
- 动态 fallback 分组名：`App.tsx:293` 已是 `effectiveCategory || "More"`（约定正确，非 "Plugins"）。

## 测试

新增 4 个测试文件：

1. `web/src/plugins/links/pages/emptyState.test.tsx` — mock `../../../ui` 桶的 `Empty`（importOriginal 展开其余导出），渲染 `LinksPage`（空 API），断言 `data-testid="shared-empty"` 出现。**断言共享组件的特征（共享 Empty 被调用），未复制空态结构进测试**。
2. `web/src/plugins/mail/pages/emptyState.test.tsx` — 同上，`MailPage` 无邮箱空态走共享 Empty。
3. `web/src/plugins/dns/pages/emptyState.test.tsx` — `DomainsPage` 无数据时 `shared-empty` **恰好 1 个**（右栏引导卡），且左栏残留文本（"add your first domain on the right"）**不出现** —— 直接守卫 #4 回归。
4. `web/src/components/ui/FormError.test.tsx` — `formErrorMessage` 单测：已知 status（403/429/500）→ 本地化 key；**未知 status（409/418）→ 回落后端原文**；字符串直通。`FormError` 渲染层同样覆盖。

**变异验证**（`file:line → 哪个用例变红`）：

| 变异位置 | 改法（仍可编译） | 变红用例 |
|---|---|---|
| `web/src/plugins/links/pages/index.tsx:196-206` | 空态分支加 `&& false`（短路共享 Empty 渲染） | `LinksPage empty state > renders the shared Empty when there are no links` |
| `web/src/plugins/dns/pages/index.tsx:195` | 左栏 `null` 分支改回手写纯文本 div（模拟 #4 回归） | `DomainsPage empty state > renders exactly one shared Empty…`（shared-empty 计数 + 残留文本断言双双失败） |
| `web/src/components/ui/FormError.tsx:26` | `if (key)` 改 `if (key && false)`（短路映射） | `formErrorMessage > maps known failure statuses…`、`FormError > renders the localized copy for a mapped status` |
| `web/src/components/ui/FormError.tsx:27` | `return err?.message ?? ""` 改 `return "Something went wrong"`（吞未知错误） | `formErrorMessage > falls back to the backend's original message…`、`FormError > renders the backend original for an unmapped status` |

每次变异确认变红后已改回原代码。

## 验证

```bash
unset http_proxy
cd web
pnpm exec tsc --noEmit   # ✅ 通过
pnpm test                # ✅ 110 用例：109 通过；1 失败为既有问题（见下）
pnpm i18n:audit          # ✅ 通过（含 OCTARQ_PRO_DIR=octarq-pro 双跑）
pnpm build               # ✅ 通过
```

**既有失败说明**：`web/src/brandRefresh.test.tsx > refreshBrand re-reads the brand… > applies the next workspace's accent` 超时失败。已验证与本批改动无关：`git stash -u` 后在干净 base（`7c127fc`）上单独运行同样失败（3 通过 1 超时）。brand/api 文件本批未触碰。已在报告中如实记录，未隐藏。

注：`pnpm build` 会重建 `webembed/dist`（Go 侧嵌入的 dashboard 产物）；按 CLAUDE.md 约定**未提交**，已 `git checkout`/`git clean` 还原。

## 截图证据（实跑，非 CI）

先 `pnpm build` 重建 bundle，再启动全新实例（临时 sqlite，`OCTARQ_LISTEN=:8792`，注册空 org 保证 links/mail/dns 均无数据），用共享工具截图。主题证明为脚本打印的 `document.documentElement.className`（`""`=light，`"dark"`=dark），非目测：

| 截图 | 内容 | 主题证明 |
|---|---|---|
| `.agy-specs/uxrest-shots/admin-links-light.png` | Links 页左栏共享 Empty（icon + "no short links yet" + host mono + New Link 按钮） | `html.class=""` |
| `.agy-specs/uxrest-shots/admin-links-dark.png` | 同上（dark） | `html.class="dark"` |
| `.agy-specs/uxrest-shots/admin-mail-light.png` | Mail 页共享 Empty（icon + "No mailbox configured yet" + New Mailbox 按钮）+ 顶部引导卡 | `html.class=""` |
| `.agy-specs/uxrest-shots/admin-mail-dark.png` | 同上（dark） | `html.class="dark"` |
| `.agy-specs/uxrest-shots/admin-domains-light.png` | DNS 页右栏唯一引导卡（Globe + "Add your first domain" + Connect Provider），左栏无残留文本 | `html.class=""` |
| `.agy-specs/uxrest-shots/admin-domains-dark.png` | 同上（dark） | `html.class="dark"` |

三张图可并排比较：links/mail 左栏空态与 dns 右栏引导卡同为共享 `Empty` 的玻璃卡（icon 槽 + reason + detail + action 槽），结构一致。

**DOM 级复核**（截图为证之外，Playwright 直连页面断言，临时脚本已删）：三页各自 `shared-Empty` 计数均为 1，dns 页无左栏残留文本；三页文本内容与上表一致。

---

*交付：本报告 + 代码改动在 `fix/uxrest` 分支提交。未 push，未开 PR。*

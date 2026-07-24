# 任务:SSO + Audit 设置页(Pro 前端插件包)—— 派给 agy(Gemini)

你是 agy。为已合并的两个后端 Pro 插件(modules/sso、modules/audit)各做一个 React 前端插件包。严格照 `packages/plugin-licensing` 的结构克隆改造。

## 工作区 & 分支
- 仓库:`/Volumes/PHD/code/octarq-pro`。**Claude 正在另一个 worktree 改 Go 后端菜单,你只碰 `packages/` 和两个 manifest JSON,不碰任何 `.go` / `modules/`。**
- `cd /Volumes/PHD/code/octarq-pro && git fetch origin && git checkout -b feat/sso-audit-ui origin/main`

## 模板:严格参考 `packages/plugin-licensing/`
先读这几个文件摸清结构与写法:
- `packages/plugin-licensing/package.json`(命名 `@octarq-org/plugin-licensing`、tsup build、peerDeps sdk/react)
- `packages/plugin-licensing/src/index.ts`(UIPlugin:name + routes[path→lazy page] + i18n;**不写 menu**——菜单由后端 MenuProvider 驱动)
- `packages/plugin-licensing/src/page.tsx`、`src/i18n.ts`
- `packages/plugin-licensing/tsconfig.json`

## 交付物 1:`packages/plugin-sso/`
镜像 licensing 结构。UIPlugin:`name: "sso"`,route `path: "/sso"` → lazy `./page`。
页面 = SSO(OIDC)配置表单,调用后端已实现的 API:
- `GET /api/sso/config` → 返回 `{enabled, issuer, clientId, secretSet, allowedDomain, redirectUri}`(secret 不返回)。
- `PUT /api/sso/config` → body `{enabled, issuer, clientId, clientSecret?, allowedDomain?}`(clientSecret 留空=保留原值)。
页面要点:
- 用 `@octarq-org/plugin-sdk` 的 UI 组件(ScreenWrap / PageHeader / GlassCard / Button / Field / useTranslation,照 licensing page.tsx 用法)。
- **用原生 `fetch`**(不要依赖 @octarq-org/api-client 的生成函数,避免生成耦合);处理 **402**(未授权 Pro → 用 SDK 的 LockedFallback/升级提示)和错误内联显示。
- 展示 `redirectUri` 并提示"把这个回调 URL 注册到你的 IdP";`secretSet` 为 true 时 client secret 输入框显示占位"已设置,留空保留"。
- 表单字段:enabled(开关)、issuer(URL)、clientId、clientSecret(password)、allowedDomain(可选)。保存后 toast/inline 成功。

## 交付物 2:`packages/plugin-audit/`
UIPlugin:`name: "audit"`,route `path: "/audit"` → lazy `./page`。
页面 = 审计日志浏览 + 导出,调用:
- `GET /api/audit/logs?action=&targetType=&actorId=&since=&until=&limit=&offset=` → `{logs:[{id,createdAt,actorId,action,targetType,targetId,ip,meta}], total}`。
- `GET /api/audit/export?<同上过滤>` → CSV 下载(直接 `window.location` 或 `<a download>` 指向该 URL 即可)。
页面要点:表格展示 logs(时间/actor/action/target/ip),顶部过滤(action、targetType、日期范围),分页(limit/offset + total),"Export CSV"按钮。同样处理 402。SDK 组件保持风格统一。

## 两个 manifest 都要登记(否则 dashboard 不加载)
1. `octarq.plugins.json`(生产,发布包):在 plugins 数组加 `"@octarq-org/plugin-sso@^0.1.0"` 和 `"@octarq-org/plugin-audit@^0.1.0"`。
2. `octarq.plugins.dev.json`(本地源码):加 `{ "from": "../../octarq-pro/packages/plugin-sso/src" }` 和 audit 同理。
两个文件保持顺序/风格一致。

## 验收硬约束(不绿不推)
- 每个包能 `cd packages/plugin-sso && pnpm build`(tsup 出 dist,含 .d.ts)成功;plugin-audit 同理。(**pnpm 11,禁 npm;先 `unset http_proxy https_proxy`**;需要 GitHub Packages token 装 sdk 依赖时用 `gh auth token` 设 NODE_AUTH_TOKEN。)
- TypeScript 通过(tsup --dts 不报错)。
- 不碰任何 `.go` 文件、`modules/`、`.github/`。菜单是 Claude 在后端加的,你**不要**在 UIPlugin 里写 menu。
- 路由 path 必须正好是 `/sso` 和 `/audit`(和后端菜单一致)。

## 提交 & 开 PR
- commit:`feat(web): SSO + Audit settings UI plugin packages`
- 推送:`git push -u origin feat/sso-audit-ui`
- PR:`gh pr create --repo octarq-org/octarq-pro --base main --head feat/sso-audit-ui --title "feat: SSO + Audit settings UI (plugin-sso, plugin-audit)" --body "Two Pro UI plugin packages calling the merged SSO/audit backend APIs; registered in both plugin manifests. Backend menus added separately by Claude."`

## 完成后
打印 `AGY-UI-DONE: <PR url>` 或 `AGY-UI-BLOCKED: <原因>`,然后停下。若某处卡住(如 sdk 依赖装不上),先把能做的做完并在 BLOCKED 里写清卡点。

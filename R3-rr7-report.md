# R3-rr7 —— react-router v6 → v7 升级报告

分支：`feat/rr7`（worktree：`.worktrees/octarq-rr7`，未动 `octarq` 主仓）
规格：`R3-rr7.md`。只动 `web/`，无任何 Go 文件改动。

## 升级前后版本

| 包 | 升级前（lockfile 实际解析） | 升级后 |
|---|---|---|
| `react-router-dom` | `^6.26.2` → 6.30.4 | `^7.18.2` → 7.18.2 |
| `react-router`（传递依赖） | 6.30.4 | 7.18.2 |

`web/package.json` 一行变更：`"react-router-dom": "^6.26.2"` → `"^7.18.2"`（7.x 当前最新）。
`web/pnpm-lock.yaml` 随 `pnpm install` 更新（`web/` 是独立 pnpm 项目，自带 lockfile，根 workspace 只管 `packages/*`）。

## 官方迁移指引依据（不是凭记忆改）

依据 React Router 官方仓库 CHANGELOG（`remix-run/react-router`，context7 拉取）中的 v7 发布条目：

- **`react-router-dom` 在 v7 仍作为 `react-router` 的 re-export 发布**（v8 才移除）——现有 `from "react-router-dom"` 导入无需迁移。
- v7 最低要求 `node@20`、`react@18`/`react-dom@18`。本仓 React 18.3.1、Node 24.12.0（CI 同 pnpm 9.15.4），满足。
- v7 将 v6 的 future flags 收为默认行为：`v7_startTransition`、`v7_relativeSplatPath`、`v7_fetcherPersist`、`v7_normalizeFormMethod`、`v7_partialHydration`、`v7_skipActionStatusRevalidation`，以及 Remix v2 的 v3_* 系列。
- v6 的 `json`/`defer` 等 data API 从 DOM 包移除——本仓未使用。

## 改了哪些 API 调用点

**源码零改动。** grep 全部 37 个 `react-router` 导入点后逐一核对，v7 下无任何调用点需要修改：

- 导入面：全部来自 `react-router-dom`（`BrowserRouter`、`Routes`/`Route`、`Navigate`、`useNavigate`、`useLocation`、`useSearchParams`、`useHref`、`Link`、`NavLink`、测试里的 `MemoryRouter`）。v7 中 `react-router-dom` 是 re-export 包，全部继续可用。
- 全仓 grep `useMatches|useParams|useRoutes|Outlet|useLoaderData|useFetcher|useSubmit|useActionData|createBrowserRouter|RouterProvider` = **无匹配**——纯声明式 `<Routes>` 模式，不涉及 v7 变化最大的 data-router 面。
- v7 默认的 `v7_startTransition`：本仓无依赖路由时序的状态假设，React 18.3 原生支持，无影响。
- `v7_relativeSplatPath`：仅影响 splat 路由内的相对导航；本仓 splat 用法是 `<Route path="/settings/*">` + 内嵌 `<Routes>`（子路由全部绝对路径）与 catch-all `<Route path="*">`（无相对链接），无影响。
- 路由排序（静态段 > splat）被 `web/src/plugins/settingsRoute.test.tsx` 显式钉住（`/settings/demo` 必须赢过 `/settings/*`），升级后该测试通过，行为未变。

改动文件清单：
- `web/package.json`（版本号）
- `web/pnpm-lock.yaml`（lockfile）

## `/status` basename 特例是怎么保住的

`web/src/main.tsx` 的 basename 切换逻辑**一字未动**：

```ts
const routerBasename =
  window.location.pathname === "/status" || window.location.pathname === "/status/" ? "/" : "/admin";
```

`BrowserRouter basename` 在 v7 中行为不变（URL 不以 basename 开头则不渲染），状态页在裸 `/status` 由后端返回同一份 index.html，`basename="/"` 让路由渲染出 StatusPage；其余一切保持 `/admin`。实跑验证：`/status` 截出的是公开状态页（见截图证据），说明该特例在 v7 下工作正常。

## plugin-sdk 是不是 breaking、changeset

**结论：不是 breaking，不需要 changeset。** 依据：

1. `packages/plugin-sdk/src/` 与 `web/src/plugin-sdk/` 全量 grep `react-router|useNavigate|useHref|NavLink|Router` = **零匹配**；`grep "from \"react-router"` 同样为零。
2. 对外暴露的唯一路由相关类型是 `UIRoute`（`packages/plugin-sdk/src/contract/types.ts`）：`path: string`（绝对 admin 路径的纯字符串）+ `Component`，与 react-router 的 API 完全解耦，v7 不改变其语义。
3. `packages/plugin-sdk/package.json` 无 react-router 依赖（peerDependencies 仅 `react`/`react-dom`）。

规格里"改了源码不加 changeset = 永不发布"的门禁针对发布包源码变更；本次未改 plugin-sdk 源码，也未改其类型契约，故不加 changeset。若未来 `UIRoute` 引入 router 类型再评估。

## `pnpm audit` 前后对比

**升级前**（临时目录以 `react-router-dom@^6.26.2` 复现，`pnpm audit --prod --registry=https://registry.npmjs.org` 完整输出）：

```
3 vulnerabilities found
Severity: 3 moderate

│ moderate │ React Router: Open redirect via backslash in <Link>        │
│           │ and useNavigate (CVE-2025-68470 bypass)                   │
│ Package   │ react-router                                               │
│ Vulnerable│ >=6.0.0 <7.18.0                                            │
│ Patched   │ >=7.18.0                                                   │

│ moderate │ React Router: Open redirect leading to XSS                 │
│ Package  │ react-router-dom                                            │
│ Vulnerable│ >=6.30.2 <=6.30.4                                          │
│ Patched  │ >=6.30.5                                                    │

│ moderate │ React Router: Arbitrary Constructor Injection via           │
│          │ deserializeErrors() in React Router SSR Hydration           │
│ Package  │ react-router                                                │
│ Vulnerable│ >=6.4.0 <7.18.0                                            │
│ Patched  │ >=7.18.0                                                    │
```

**升级后**（`web/` 内 `pnpm audit --prod --registry=https://registry.npmjs.org` 完整输出）：

```
No known vulnerabilities found
```

3 个 moderate 全部消失——三者 patch 版本都要求 `>=7.18.0`，6.x 分支无修复版，升 v7.18.2 是唯一解法。仓库默认 registry 是 npmmirror 无 audit 端点，故显式 `--registry=https://registry.npmjs.org`。

## 验证命令结果

| 命令 | 结果 |
|---|---|
| `pnpm install`（pnpm 9.15.4，未升 10/11） | 成功 |
| `pnpm exec tsc --noEmit` | 通过，exit 0 |
| `pnpm test` | 26 files / 96 tests 全过（首轮 `brandRefresh.test.tsx` 一次 5s 超时，隔离重跑与二轮全量均通过——该测试不导入 router，系整机 collect 耗时 117s 的负载抖动） |
| `pnpm i18n:audit` | 全部通过（1118 keys / 5 locales / 11 menu ids） |
| `pnpm build` | 成功（12.79s，`webembed/dist` 已还原不提交） |
| `pnpm audit --prod` | 见上，0 漏洞 |

## 截图证据（先 build 后重启服务，截的是新包）

截图工具：`.agy-specs/shot.mjs`（自带主题证明与登录失败检测，未重造）。服务流程：`pnpm build`（重建 `webembed/dist`）→ 重启后端 `go run .`（`go:embed` 编译期嵌入新 dist，`OCTARQ_LISTEN=:8791`）→ shot.mjs 登录 `admin@example.com` 逐路由截图。

文件（仓库相对路径）+ 主题证明（`document.documentElement.className`，`""`=light，`"dark"`=dark）：

| 截图 | html.class |
|---|---|
| `R3-rr7-shots/login-light.png` | `""` |
| `R3-rr7-shots/login-dark.png` | `dark` |
| `R3-rr7-shots/admin-overview-light.png` | `""` |
| `R3-rr7-shots/admin-overview-dark.png` | `dark` |
| `R3-rr7-shots/admin-links-light.png` | `""` |
| `R3-rr7-shots/admin-links-dark.png` | `dark` |
| `R3-rr7-shots/admin-settings-general-light.png` | `""` |
| `R3-rr7-shots/admin-settings-general-dark.png` | `dark` |
| `R3-rr7-shots/status-light.png` | `""` |
| `R3-rr7-shots/status-dark.png` | `dark` |

shot.mjs 未抛登录失败错误（所有鉴权页都不是登录页）。另做了 DOM 级断言（同服务、登录后逐页抓正文）：四个路由均渲染真实页面、`main` 可视高度 >100px、无 "That page isn't in this build"/routeUnavailable 等 fallback 标记：

- `/admin/overview`：侧栏（Operations/Infrastructure/MARKETING/MESSAGING/SECURITY/SYSTEM）+ Overview 页 + 设置进度卡
- `/admin/links`：Links 插件页（"+ New Link"、Links/Settings/Active 页签、空列表空态——localhost 播种缺陷是已知问题，非本次引入）
- `/admin/settings/general`：设置面板（General/Features/Members/Webhooks/Alerts/ACCOUNT/INSTANCE）+ Workspace Profile 表单
- `/status`：公开状态页（"All Systems Operational" + 子系统表）——**basename 特例在 v7 下验证通过**

路由是全站骨架，以上 = 路由 v7 下实跑可用。

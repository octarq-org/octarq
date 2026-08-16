# R3-registry 报告 —— 前端插件注册表同名冲突不再静默

分支：OSS `feat/registry` / octarq-pro `feat/audit-plugin-rename`（未 push，未开 PR）

## 1. 去重逻辑的真实位置和语义（自己读代码确认，非照抄背景）

**位置**：`packages/plugin-sdk/src/contract/registry.ts`（注意：不在
`web/src/plugin-sdk/` 里——那是 app 侧 facade，只做 `export * from
"../../../packages/plugin-sdk/src"` 转发；真正的注册表在 `packages/plugin-sdk/src/contract/registry.ts`）。

**原实现**（`packages/plugin-sdk/src/contract/registry.ts:16`）：

```ts
if (REGISTRY.some((p) => p.name === plugin.name)) return;
REGISTRY.push(plugin);
```

- **按 `name` 去重**，不是按 route。
- **先注册者赢**：`return` 直接丢弃后来者，整个 plugin（routes/widgets/areas/i18n
  全部）都不进注册表，**无任何 warning/error**。
- **注册顺序**：`web/src/main.tsx` 先 `import "./plugins/core"`（abuse/audit/help/notifyCore
  无条件注册），后 `import "#octarq-plugins"`（manifest 虚拟模块）。所以 core 的
  `audit`（`web/src/plugins/core/audit.ts`，name `"audit"`，route `/audit`）永远先注册，
  octarq-pro 的 `@octarq-org/plugin-audit`（name `"audit"`，route `/audit`）永远被静默丢弃。
- 后端 `app/preflight.go` 的 `preflightNameCollisions` 是**启动即拒绝**；前端此前是**静默忽略**。
  两侧行为不对称，bug 得以存活。

## 2. dev/prod 分支实现

`packages/plugin-sdk/src/contract/registry.ts`（`registerUIPlugin`）：

```ts
const existing = REGISTRY.find((p) => p.name === plugin.name);
if (existing) {
  const message =
    `UIPlugin name collision: "${existing.name}" is already registered by another plugin ` +
    `(routes: ${existing.routes.length}); the incoming plugin "${plugin.name}" ` +
    `(routes: ${plugin.routes.length}) was ignored. A plugin name is its identity — ` +
    `first registration wins, so rename one of the two plugins.`;
  if (import.meta.env.DEV) {
    throw new Error(message);
  }
  console.error(message);
  return;
}
REGISTRY.push(plugin);
```

- **dev `throw`**：把问题挡在开发者面前（对齐 Go 侧"绝不允许带病运行"的语义）。
- **prod `console.error`**：第三方插件重名不能白屏整个后台，但错误必须可见，不再静默。
  错误信息含两个插件的名字（因按 name 去重，两插件同名——消息中同时点名 existing 与
  incoming 两个注册方，并以各自 route 数区分），并给出修复指引。
- **dev/prod 判定**：`import.meta.env.DEV`，与仓库既有惯例一致（`website/src/components/ui/*.client.ts`
  均用 `import.meta.env.DEV`）。
- **类型配套**：新增 `packages/plugin-sdk/src/vite-env.d.ts`（ambient 声明
  `ImportMetaEnv`/`ImportMeta`）。包内没有 vite 直接依赖，pnpm 严格 node_modules 下
  `/// <reference types="vite/client" />` 无法解析，故自包含声明，与 `web/src/vite-env.d.ts`
  的 shape 一致（DEV/PROD/MODE/BASE_URL/SSR），保证包自身 `tsc --noEmit` 通过。

## 3. 守卫测试 + 变异验证

**测试**：`packages/plugin-sdk/src/contract/registry.test.ts`。原有幂等用例
（"second registration is ignored"）在 dev 语义下必然抛错，改写为：

- `registerUIPlugin > throws in dev when a name is registered twice, keeping the
  first registration`（`registry.test.ts:35`）：断言第二次同名注册 `toThrow(/UIPlugin
  name collision: "dup"/)`，且注册表仍只有 first、route 仍为 `/first`。

（另尝试过用 `vi.stubEnv("DEV", "false")` 补 prod 分支单测——vitest 下
`import.meta.env.DEV` 是编译期替换值，stubEnv 无法覆盖，prod 分支无法在 vitest 里切，
已移除该用例。prod 分支行为改由生产构建产物验证，见 §5。）

**变异验证**：将 `packages/plugin-sdk/src/contract/registry.ts:25` 的 `if (existing)`
短路为 `if (existing && false)`（仍能编译）：

- `registry.ts:25 → registry.test.ts:39` 用例变红：
  `throws in dev when a name is registered twice, keeping the first registration`
  （`AssertionError: expected [Function] to throw an error`）。
- 验证后已改回 `if (existing)`。

## 4. Pro 改名前后的插件名 + 路由覆盖结论

**改名**（octarq-pro，唯一允许动的地方）：

| | 之前 | 之后 |
|---|---|---|
| `octarq-pro/packages/plugin-audit/src/index.ts` | `name: "audit"` | `name: "auditPro"` |
| `octarq-pro/packages/plugin-audit/src/page.tsx` | `t("audit.*")` ×23 | `t("auditPro.*")` ×23 |

- 新名 `auditPro`：camelCase，与 octarq-pro 其他多词插件名（`inboxAi`、`llmProviders`、
  `sshKeys`）一致；与 core 注册名（abuse/audit/help/notify/dns/mail/links/hello）及全部
  Pro 插件名无冲突。
- `page.tsx` 的 23 处 i18n 前缀必须同步：`uiPluginI18n()` 按 `p.name` 键控 namespace，
  不改则 `t("audit.*")` 全部回退成 key 原文。
- `src/i18n.ts` 的 `_shared.nav.audit` **保留不变**：那是 Go 侧菜单 ID（
  `octarq-pro/modules/audit/audit.go:97` `{ID: "audit", ...}`）的 nav label 查找键
  （`navI18n.ts` 的 `nav.<id>`），不是插件名，改名不影响。
- 包名 `@octarq-org/plugin-audit`、manifest（`octarq.plugins.json` /
  `octarq.plugins.dev.json`）均按包名引用，无需改动。

**路由覆盖结论**：改名只消除了注册表冲突，**并不能让 Pro 页面真的渲染**。

- core 审计页与 Pro 增强页注册的是**同一个 `/audit` 路径**：
  core `web/src/plugins/core/audit.ts`（`/audit`）+ Pro `plugin-audit`（`/audit`）。
- 路由层由 `web/src/plugins/PluginRoutes.tsx` 的 `pluginRouteElements()` 把
  `uiPlugins()` 按注册顺序 flatMap 成 `<Route>`；React Router v6 对相同 path 的
  排名相等、按树序先声明者命中——**core 先注册（main.tsx 先组合 plugins/core），
  所以 `/audit` 永远是 core 的页面**。Pro 改名后其 `/audit` 路由仍是死路由。

**要让 Pro 页面真正生效还差什么（待决策，本批次不做）**：

core 需要一个「路由/插件可被替换」的机制，候选方案：

1. **UIPlugin 契约加 `replaces?: string[]`（按 name）或按 route 的 override 字段**：
   注册时若后注册方声明替换，注册表把先注册方的同名/同 path 条目移除或置后。
   Pro 的 `auditPro` 声明 `replaces: ["audit"]`（或 route `replace: ["/audit"]`），
   即真覆盖 core 审计页。这是对 core `UIPlugin` 契约与注册表的改动。
2. **Pro 构建排除 core 的 audit UIPlugin**：但 core 插件在 `plugins/core/index.ts`
   无条件组合，Pro manifest 无法剔除；需要 core 允许 manifest 层显式排除某 core 插件。
3. **保持现状、接受"Pro 页不渲染"**：改名后至少不再报错、不再静默，但付费用户仍看到
   core 朴素审计页——不满足产品诉求，仅作为过渡。

**推荐方案 1**（`replaces` 字段 + 注册表替换语义），需要 core 侧契约/注册表/路由三层
改动，超出本规格范围，标为**待决策**。

## 5. 验证结果

OSS（worktree `/Volumes/PHD/code/.worktrees/octarq-registry`）：

```
packages/plugin-sdk: pnpm test        27/27 passed ✓
packages/plugin-sdk: pnpm exec tsc --noEmit   ✓
web: pnpm exec tsc --noEmit            ✓
web: pnpm test                         96/96 passed ✓
web: pnpm i18n:audit                   全部通过 ✓
web: pnpm build                        ✓
```

prod 分支行为验证（构建产物 `webembed/dist/assets/index-*.js`）：

- 产物含 `console.error(t)`（消息串后紧跟 `console.error(t)`）→ prod 分支在；
- `grep -c 'throw new Error("UIPlugin'` = 0 → dev 的 throw 分支已被摇树。
- 即生产构建中同名冲突走 `console.error` + first-wins，且不影响构建（第三方插件重名不会白屏）。

（构建会刷新 `webembed/dist`，已按仓库约定 `git checkout -- webembed/dist` +
`git clean -f webembed/dist/` 还原——该目录由 CI 在合并后刷新，不手动提交。）

octarq-pro（`/Volumes/PHD/code/octarq-pro`）：

```
GOWORK=off go build ./...             ✓ （go.work 被 gitignore，本地默认会顶掉 pin 的
                                        版本，故按规格用 GOWORK=off 验证）
pnpm -r --if-present build            全部 Build success ✓（含 plugin-audit tsup + dts）
```

git 变更范围：

- OSS：`packages/plugin-sdk/src/contract/registry.ts`、
  `packages/plugin-sdk/src/contract/registry.test.ts`、
  `packages/plugin-sdk/src/vite-env.d.ts`（新增）、`R3-registry-report.md`（新增）
- octarq-pro：`packages/plugin-audit/src/index.ts`、`packages/plugin-audit/src/page.tsx`
  （`packages/plugin-audit/dist/` 已被 gitignore，构建产物不入库）

## 6. 跨仓合并顺序

**OSS 先行，Pro 后跟**：`feat/registry` 必须先合入 octarq 主干，随后
`feat/audit-plugin-rename` 才能合入 octarq-pro。原因：OSS 的 registry 一旦上线，
Pro 旧版 `name: "audit"` 在 dev 会直接 throw、prod 会 console.error——Pro 的改名
必须紧随其后落地，否则 Pro 升级即炸。反向（Pro 先合）会让 Pro 的 `auditPro` 依赖
一个还不存在的注册表语义，且旧 registry 下改名后的 `/audit` 路由仍与 core 撞名（虽然
这次是路由层静默死路由，不是注册表冲突），没有意义。**故合并顺序固定为 OSS → Pro。**

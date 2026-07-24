# 任务:充实文档站(加入系统设计 / 插件设计)—— 派给 agy(Gemini)

现在的 docs 站(octarq/website,Starlight)只有 5 页,太简陋。把仓库里已有的架构设计文档搬进去,组织成分组侧边栏。**只搬运+组织+适配,不发明新架构内容。**

## 工作区 & 分支
- 仓库:`/Volumes/PHD/code/octarq`,docs 站在 `website/`。
- `cd /Volumes/PHD/code/octarq && git fetch origin && git checkout -b feat/docs-architecture origin/main`
- 只碰 `website/`(新增 md 页 + 改 `website/astro.config.mjs` 的 sidebar)。**不碰 Go、web/、docs/*.md 源文件(它们是素材,只读)。**

## 素材(在 `docs/` 下,只读搬运)
- `docs/PLUGIN-ARCHITECTURE.md` — 插件架构总览
- `docs/PLUGIN-COMPOSABILITY.md` — 插件可组合性 / plugin.Context 能力
- `docs/PLUGIN-COMPOSITION-UNIFICATION.md` — 组合统一(Core 与 Pro 同一 a.Use 机制)
- `docs/CORE-PLUGIN-EXTRACTION.md` — 把 links/mail/dns 从 god-Handler 抽成 Core 插件
- `docs/CORE-DECOUPLING-AUDIT.md` — 核心解耦审计
- `docs/PUBLISHING.md` — 发布/组合二进制
- `docs/ACCESSIBILITY.md` — 前端无障碍规范

## 交付:分组侧边栏 + 新页
1. 在 `website/src/content/docs/` 下新建页面(每个素材→一页,frontmatter 加 title/description)。建议路径:
   - `architecture/overview.md`(来自 PLUGIN-ARCHITECTURE)
   - `architecture/plugin-context.md`(来自 PLUGIN-COMPOSABILITY)
   - `architecture/composition.md`(来自 PLUGIN-COMPOSITION-UNIFICATION)
   - `architecture/core-plugins.md`(来自 CORE-PLUGIN-EXTRACTION + CORE-DECOUPLING-AUDIT,可合并成一页或两页)
   - `guides/publishing.md`(来自 PUBLISHING)
   - `contributing/accessibility.md`(来自 ACCESSIBILITY)
   搬运时:去掉纯内部/过时的实现细节(如 PR 编号、临时债务清单),保留对使用者/插件作者有价值的设计说明。Starlight 用 Markdown/MDX;代码块保留。
2. 改 `website/astro.config.mjs` 的 `sidebar` 为**分组**结构(Starlight 用 `{ label, items: [...] }`):
   - **Start**:Overview(/)、Quickstart、Deploy
   - **Build a Plugin**:Writing a Plugin、Plugin Directory
   - **Architecture**:Overview、plugin.Context、Composition、Core Plugins
   - **Guides**:Publishing、Accessibility
   保留现有 5 页,只是归入分组。

## 验收硬约束(不绿不推)
- `cd website && pnpm install && pnpm build` 必须成功(**pnpm 9.15.4,禁 npm;先 `unset http_proxy https_proxy`**;仓库钉 pnpm 9.15.4)。
- 所有内部链接可解析(Starlight 构建会校验)。不提交 node_modules/dist。
- 不碰 Go / web/ / docs/*.md 源。

## 提交 & PR
- commit:`docs(website): add Architecture + Guides sections (system & plugin design)`
- 推送 `git push -u origin feat/docs-architecture`
- PR:`gh pr create --repo octarq-org/octarq --base main --head feat/docs-architecture --title "docs: architecture & plugin-design pages" --body "Folds the repo's design docs (plugin architecture, composability, composition, core-plugin extraction, publishing, a11y) into the Starlight docs site with a grouped sidebar. Content moved/organized from docs/*.md, no new architecture invented."`

## 完成后
打印 `AGY-DOCS-DONE: <PR url>` 或 `AGY-DOCS-BLOCKED: <原因>`,停下。

## 追加:域名修正
在 `website/astro.config.mjs` 里把 `site: 'https://docs.octarq.com'` 改成 `site: 'https://docs.octarq.org'`(域名是 octarq.org,不是 .com)。

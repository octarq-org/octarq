# 任务 T11:octarq 定价页(marketing) —— 派给 agy(Gemini)

你是 agy。严格按本规格执行。只做前端页面,不碰 Go。

## 工作区 & 分支(避免和 Claude 抢工作树)
- 仓库:`/Volumes/PHD/code/octarq-pro`(Astro marketing/docs 站在 `web/`)。**Claude 在独立 worktree 上做 Go(SSO/审计),你只碰 `octarq-pro/web/`。**
- `cd /Volumes/PHD/code/octarq-pro && git fetch origin`
- 新分支基于最新 main:`git checkout -b feat/pricing-page origin/main`
- 只在 `web/` 下新增/改动。**不要碰任何 `.go`、`modules/`、`packages/`、`.github/`。**

## 交付物:`web/src/pages/pricing.astro` —— 三档定价页
用 Astro 文件路由新建 `web/src/pages/pricing.astro`(若 `src/pages/` 不存在就建)。先读 `web/astro.config.mjs`、`web/src/components/Footer.astro`、`web/package.json` 摸清站点已用的框架/样式(Starlight? Tailwind? 原生 CSS?),**沿用现有风格**,不要引入新 UI 依赖。

三档卡片(数字是 Claude 参考市场给的**草案**,页面顶部加一行注释 `<!-- DRAFT pricing — 待 owner 确认 -->`):

| 档 | 价格 | 定位一句话 | 功能点 |
|---|---|---|---|
| **Community** | **Free** (MIT) | 自托管,永久免费 | 单二进制 + 内嵌面板;links/mail/DNS 参考插件;MCP agent-native;社区插件;无限工作区 |
| **Pro** | **$149 / 年**(每实例,自托管 license) | 一人公司的完整后台 | 含 Community 全部;**Pro 插件集:SSO/OIDC、审计日志、白标品牌、infra & finance 模块**;1 年更新;邮件优先支持。(可加一行小字:**Elite $349/年** 额外解锁 AI 模块 —— AI 收件箱/简报/over-MCP) |
| **Cloud** | **$19 / 月起**(托管) | 我们帮你托管 | 全托管 + 自动更新/备份;含 Pro 插件;自定义域名;零运维 |

要求:
- 语义化、响应式、a11y(每卡片有标题/价格/CTA 按钮);Pro 卡片高亮为"推荐"。
- CTA:Community→GitHub(https://github.com/octarq-org/octarq);Pro→占位链接 `/buy`(或 `#`);Cloud→占位 `/cloud`(或 `#`)。
- 顶部一句 pitch:"Own your back office. Start free, scale when you do."
- 页面能被现有站点导航访问(若有 nav/sidebar 配置就加一项 "Pricing" → `/pricing`;找不到就只建页面,别硬改布局)。

## 验收硬约束(不绿不推)
- `cd web && pnpm install && pnpm build` 必须成功(**用 pnpm,禁 npm;先 `unset http_proxy https_proxy`**;pro/web 用 pnpm 11,按仓库 packageManager 走)。
- 不提交 `node_modules/`、`dist/`。
- 不碰任何 `.go` 文件。

## 提交 & 开 PR
- commit:`feat(web): three-tier pricing page (draft numbers) (T11)`
- 推送:`git push -u origin feat/pricing-page`(没改 workflow,HTTPS 即可)
- PR:`gh pr create --repo octarq-org/octarq-pro --base main --head feat/pricing-page --title "feat: pricing page (T11)" --body "Three-tier pricing (Community/Pro/Cloud). Draft numbers from market reference, pending owner confirmation. pnpm build green."`

## 完成后
在 pane 打印 `AGY-T11-DONE: <PR url>` 或 `AGY-T11-BLOCKED: <原因>`,然后停下等下一个任务。

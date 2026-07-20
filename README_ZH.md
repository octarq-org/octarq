# Octarq —— 一人公司与 AI 团队的自托管后台

[![CI](https://github.com/octarq-org/octarq/actions/workflows/ci.yml/badge.svg)](https://github.com/octarq-org/octarq/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![MCP Powered](https://img.shields.io/badge/MCP-Enabled-purple.svg)](https://modelcontextprotocol.io)

[English](README.md) | [简体中文](README_ZH.md)

**Octarq 是一个用插件扩展的单 Go 二进制应用** —— 专为独立开发者、一人公司及 AI-native 敏捷团队打造的自托管运营后台。

拥有一套独立域名？Octarq 能直接为你省去三份 SaaS 账单：**带数据分析的短链接、邮件收发与路由、DNS 自动化管理** —— 每一项都作为一等插件交付，而非死板的功能硬编码。你可以像官方核心一样扩展它：**一个轻量的 Go 接口 + 一个 React 页面 = 一个全新的后台工具**。并且由于 Octarq 原生支持 **MCP**，你编写的每一个插件都能被 AI Agent（如 Claude Code、Cursor、Claude Desktop）直接驱动。

> 单二进制。无 CGO。默认 SQLite 存储。嵌入式 React 控制面板 (`go:embed`)。无需 Fork 自由扩展。

---

## 为什么选择 Octarq

把它想象成你所熟知的三个工具的交集：

- **PocketBase 式的单二进制 DX** —— 抽空半天就能扩展开发；
- 结合 **Dub 式的短链场景** + 真实的域名/邮件/DNS 基础设施；
- 加上 **n8n 式的连接器生态**（支持 Telegram、Webhooks、SMS 等插件）；
- 并且所有能力在 **MCP 协议下原生 agent-native**。

Octarq 绝非一个顺便做邮箱的短链工具，它是一个**框架**：短链接、邮件、DNS 是用于证明该架构模型的官方参考插件，而整个生态将随一人公司自托管所需的下一个能力自然延伸。

---

## 开箱即用（官方参考插件）

默认构建中内嵌了以下插件，使 Octarq 在第一分钟就落地可用：

- **🔗 短链接 (Links)** —— 自定义/随机 slug，地理/设备/OS/语言动态分流路由，过期机制与点击上限，过期 URL 兜底，带机器人过滤的时序数据分析，UTM 构造器，二维码，标签管理（支持可选 MaxMind GeoIP）。
- **✉️ 邮箱路由 (Mail)** —— 基于 Cloudflare Email Routing 的 Serverless 邮件接收（无需开启 25 端口，无垃圾邮件守护进程），Catch-all 自动建箱，完整邮件客户端（读件/回复/通过自定义 SMTP relays 发信，下载原始 `.eml`），按需 AI 邮件摘要（BYO key）。
- **🌐 DNS 管理 (DNS)** —— Cloudflare 与 DNSPod CRUD 操作，短链接与邮件认证 presets（MX/SPF/DKIM），原生备注/注释映射。
- **🏢 工作区与 RBAC (Workspaces & RBAC)** —— 隔离的多租户组织，服务端强制角色校验，邀请与设密流程，SHA-256 哈希 Bearer API tokens。

上述每一个能力均为一个 Go `plugin.Plugin` + 前端 `UIPlugin` —— 与你自己编写插件时使用的完全是同一套缝隙。

---

## Agent-Native：你的插件即 MCP 工具

Octarq 内置了 **MCP 服务器**（`octarq mcp`，支持 stdio 及 SSE/stream 模式），AI 助手（如 Claude Code）可以直接读取并查询你的实例能力 —— `list_links`、`list_mailboxes`、`list_domains`、`export_data`，以及一个**受控只读 SQL 查询工具**（限制仅 `SELECT`/`WITH` 只读事务，结果自动分页并屏蔽敏感字段）。

重点不在于“我们加入了 AI”，而在于**框架级别的管线机制**：实现可选 `MCPProvider` 接口的插件会自动将其工具暴露给所有连接的 AI Agent —— 无需额外胶水代码。编写一个插件，你的 AI Agent 就能直接驱动它。

```
📬 收到新的 OTP 验证码邮件 ──▶ 你编写的 10 行插件 (OnEmail hook)
                                ├─ 自动生成短链接 (links 服务)
                                └─ 暴露为 MCP 工具 ──▶ Claude Code 直接消费使用
```

---

## 快速开始

```bash
git clone https://github.com/octarq-org/octarq.git && cd octarq
cp .env.example .env          # set OCTARQ_SECRET_KEY and OCTARQ_ADMIN_PASSWORD
docker compose up -d
```

打开 `http://localhost:8080` 并登录。这就是完整栈 —— 控制面板、API、重定向器、MCP —— 统一运行在由 SQLite 驱动的单个容器中。

偏好约 19MB 的 `scratch` 极轻量镜像或源码构建？参阅 `make release` 与 [`deploy/`](deploy/)。

---

## 插件扩展开发（编写一个插件）

一个插件是一个包含前后端两个半边的仓库，在**编译期组合**（类似 `xcaddy` 模型 —— 选择插件，编译为一个二进制）：

```go
// backend: 实现包含 3 个方法的接口，免费获得 DB/鉴权/审计/DNS/发信/缓存等能力
func (Plugin) Name() string          { return "hello" }
func (Plugin) Models() []any         { return nil }
func (Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
    mux.Handle("GET /api/hello/ping", ctx.Guard(http.HandlerFunc(pong)))
}
```

```ts
// frontend: 基于共享 SDK 编写 React 页面 — 样式与应用统一，自带无障碍支持
export const helloPlugin: UIPlugin = {
  name: "hello",
  routes: [{ path: "/hello", Component: lazy(() => import("./Page")) }],
  menu:  [{ id: "hello", label: "Hello", path: "/hello", icon: "👋", category: "Workspace" }],
};
```

- **绝不 Fork，绝不 import `internal/*`** —— 一切能力尽在 `plugin.Context`（DB, Guard, Encrypt, Audit, Notify, SendMail, OnEmail, DNS, cache, geo, webhooks, job queue…），且支持向下兼容演进。
- **路由自动门禁**（未启用时响应 404），**插件间通过 Service Registry 通信**（无直接 Cross-Import），**模型自动 AutoMigrate**。
- **无须修改代码即可组合自定义二进制** —— xcaddy 式驱动：
  ```bash
  OCTARQ_PLUGINS='[{"go":"github.com/you/octarq-plugin-foo","npm":"@you/octarq-plugin-foo"}]' make plugin-build
  ```

**从模板快速开始** → [octarq-plugin-template](https://github.com/octarq-org/octarq-plugin-template) · **详细开发指南** → [docs/PLUGINS.md](docs/PLUGINS.md)

*Octarq 自己的商业 Pro 版也只是基于这同一套公开接口构建的另一组插件 —— 没有任何社区做不到的事情。*

---

## 配置指南

- **通过 Cloudflare Worker 接收邮件** —— 部署 [`deploy/cloudflare-email-worker.js`](deploy/cloudflare-email-worker.js)，配置 catch-all 路由，并开启 *Accept email*。
- **GeoIP 地理位置统计** —— 设置 `OCTARQ_MAXMIND_LICENSE_KEY`（免费），Octarq 会自动下载并热加载 GeoLite2。参阅 [`deploy/GEOIP.md`](deploy/GEOIP.md)。
- **Claude Desktop 配置 MCP** —— 在 `claude_desktop_config.json` 中指向 `octarq mcp`。

<details>
<summary>Claude Desktop MCP 配置</summary>

```json
{ "mcpServers": { "octarq": {
  "command": "/path/to/octarq", "args": ["mcp"],
  "env": { "OCTARQ_DB_PATH": "/path/to/octarq.db" }
}}}
```
</details>

---

## 本地开发

```bash
OCTARQ_SECRET_KEY=dev OCTARQ_ADMIN_PASSWORD=dev go run .   # 后端 API :8080
make dev                                                  # Vite 前端开发服务 (代理 /api)
go test ./... -race
```

---

## 致谢 (Credits)

设计灵感与思路借鉴自：
- [sink](https://github.com/ccbikai/sink),
- [wr.do](https://github.com/oiov/wr.do),
- [dub](https://github.com/dubinc/dub),

以及由以下项目设立的极致开发者体验 (DX) 标准：
- [PocketBase](https://github.com/pocketbase/pocketbase).

## 开源协议

基于 [MIT 协议](LICENSE) 开源。框架部分保持宽松代码许可证；商业版则为构建于其上的闭源插件集。

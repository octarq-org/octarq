<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/banner-dark.png">
    <img src="assets/banner-light.png" alt="Octarq" width="620">
  </picture>
</p>

<p align="center">
  <b>一个 Go 二进制。短链、邮件、DNS 开箱即用；用插件扩展它 —— 并让 AI Agent 通过 MCP 驱动这一切。</b>
</p>

<p align="center">
  <a href="https://github.com/octarq-org/octarq/actions/workflows/ci.yml"><img src="https://github.com/octarq-org/octarq/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/octarq-org/octarq/actions/workflows/ci.yml"><img src="https://img.shields.io/badge/coverage-≥90%25-brightgreen.svg" alt="Coverage: ≥90%"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/octarq-org/octarq?logo=go&label=Go" alt="Go version"></a>
  <a href="https://pkg.go.dev/github.com/octarq-org/octarq"><img src="https://pkg.go.dev/badge/github.com/octarq-org/octarq.svg" alt="Go Reference"></a>
  <a href="https://modelcontextprotocol.io"><img src="https://img.shields.io/badge/MCP-enabled-8b5cf6.svg" alt="MCP enabled"></a>
</p>

<p align="center">
  <a href="https://octarq.org">官网</a> ·
  <a href="https://docs.octarq.org">文档</a> ·
  <a href="#快速开始">快速开始</a> ·
  <a href="#插件扩展开发编写一个插件">编写插件</a> ·
  <a href="https://github.com/octarq-org/octarq-plugins">插件仓库</a> ·
  <a href="README.md">English</a>
</p>

---

## Octarq 是什么？

Octarq 是**面向独立开发者、一人公司与 AI-native 小团队的自托管运营后台** —— 那些原本要靠三份 SaaS 账单拼出来的后台，收进一个二进制里。

拥有一套独立域名？Octarq 直接给你**带数据分析的短链接、邮件收发与路由、DNS 自动化管理** —— 每一项都作为一等插件交付，而非死板的功能硬编码。你可以像官方核心一样扩展它：**一个轻量的 Go 接口 + 一个 React 页面 = 一个全新的后台工具**。并且由于 Octarq 原生支持 **MCP**，你编写的每一个插件都能被 AI Agent（如 Claude Code、Cursor、Claude Desktop）直接驱动。

> 单二进制。无 CGO。默认 SQLite 存储。嵌入式 React 管理后台 (`go:embed`)。无需 Fork 自由扩展。

<p align="center">
  <img src="assets/screenshot-overview.png" alt="Octarq 管理后台 —— 点击分析、Top 短链、地域与设备分布、最近邮件" width="900">
</p>

---

## 快速开始

零配置 —— 一条命令，无需 `.env`：

```bash
# 生产推荐零配置（自动生成高强度密钥与 admin 密码）：
docker run -p 8080:8080 -v octarq-data:/data ghcr.io/octarq-org/octarq:latest

# 或极速本地体验（使用 admin / admin 快速登录）：
docker run -p 8080:8080 -e OCTARQ_SECRET_KEY=dev -e OCTARQ_ADMIN_PASSWORD=admin -v octarq-data:/data ghcr.io/octarq-org/octarq:latest
```

未设置环境变量时，首次启动 Octarq 会自动生成密钥与初始 `admin` 密码（两者都持久化在 `/data`，并在日志里打印一次），默认用 SQLite 起来。打开 `http://localhost:8080`，从容器日志里拿到密码（或使用上述预设的 `admin`）即可登录。邮件、DNS、GeoIP 全部 opt-in —— 之后在 **设置** 里配置，启动时什么都不需要。

这就是完整栈 —— 管理后台、API、重定向器、MCP —— 统一运行在单个容器中。

<details>
<summary>想从源码构建？</summary>

```bash
git clone https://github.com/octarq-org/octarq && cd octarq
make release                                                # 构建 ./octarq，管理后台已嵌入
OCTARQ_SECRET_KEY=… OCTARQ_ADMIN_PASSWORD=… ./octarq        # 监听 :8080
```

想自己管理密钥/密码时，显式设置 `OCTARQ_SECRET_KEY` / `OCTARQ_ADMIN_PASSWORD`（见 `.env.example` 与 `docker compose`）。最小体积镜像见 [`deploy/Dockerfile.binary`](deploy/Dockerfile.binary)。
</details>

---

## 为什么选择 Octarq

- **一个二进制，一个下午。** 下载、运行、扩展。无需安装运行时，无需拼接服务，无 CGO。
- **是真基础设施，不是玩具。** 带地理/设备分流与机器人过滤的短链分析、能读能回的自定义域名邮箱、对接 Cloudflare 与 DNSPod 的 DNS 自动化。
- **是框架，不是套餐。** 短链、邮件、DNS 都写在你同样能用的那套公开接口之上，它们没有任何特权。
- **原生面向 Agent。** 每个插件的能力无需额外胶水即可通过 MCP 交给你的 AI Agent 驱动。
- **完全属于你。** 你的服务器、你的数据库、你的域名，MIT 协议。

---

## 开箱即用（官方参考插件）

默认构建中内嵌了以下插件，使 Octarq 在第一分钟就落地可用：

| 插件 | 能力 |
| --- | --- |
| **🔗 短链接 Links** | 自定义/随机 slug，地理/设备/OS/语言动态分流路由，过期机制与点击上限，过期 URL 兜底，带机器人过滤的时序数据分析，UTM 构造器，二维码，标签管理（支持可选 MaxMind GeoIP）。 |
| **✉️ 邮箱路由 Mail** | 基于 Cloudflare Email Routing 的 Serverless 邮件接收（无需开启 25 端口，无垃圾邮件守护进程），Catch-all 自动建箱，完整邮件客户端（读件/回复/通过自定义 SMTP relays 发信，下载原始 `.eml`），按需 AI 邮件摘要（BYO key）。 |
| **🌐 DNS 管理** | Cloudflare 与 DNSPod CRUD 操作，短链接与邮件认证 presets（MX/SPF/DKIM），原生备注/注释映射。 |

<details>
<summary>看看短链与邮件插件的实际界面</summary>

<p align="center">
  <img src="assets/screenshot-links.png" alt="短链插件 —— slug、路由域名、标签、点击上限与过期设置" width="880">
  <br><br>
  <img src="assets/screenshot-mail.png" alt="邮件插件 —— 跨自定义域名邮箱的收件箱" width="880">
</p>
</details>

其余能力 —— 鉴权、工作区与 RBAC、审计日志、通知、任务队列、Webhooks、备份 —— 属于核心，且对你编写的每一个插件开放。

---

## Agent-Native：你的插件即 MCP 工具

Octarq 内置了 **MCP 服务器**（`octarq mcp` 走 stdio；服务器自身在 `/api/mcp/sse` 与 `/api/mcp/stream` 提供 SSE 与 Streamable HTTP），AI 助手（如 Claude Code）可以直接读取并查询你的实例能力 —— `list_links`、`list_mailboxes`、`list_domains`、`export_data`。所有工具均为只读，且严格限定在调用方自己的工作区内。

重点不在于"我们加入了 AI"，而在于**框架级别的管线机制**：实现可选 `MCPProvider` 接口的插件会自动将其工具暴露给所有连接的 AI Agent —— 无需额外胶水代码。编写一个插件，你的 AI Agent 就能直接驱动它。

```
📬 收到新的 OTP 验证码邮件 ──▶ 你编写的 10 行插件 (OnEmail hook)
                                ├─ 自动生成短链接 (links 服务)
                                └─ 暴露为 MCP 工具 ──▶ Claude Code 直接消费使用
```

<p align="center">
  <img src="assets/octarq-demo.gif" alt="Octarq agent-native 演示 —— 在终端里通过 MCP 驱动每一个插件" width="720">
</p>

<details>
<summary>Claude Desktop MCP 配置</summary>

```json
{ "mcpServers": { "octarq": {
  "command": "/path/to/octarq", "args": ["mcp"],
  "env": { "OCTARQ_DB_DSN": "/path/to/octarq.db" }
}}}
```
</details>

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
};
```

- **绝不 Fork，绝不 import `internal/*`** —— 一切能力尽在 `plugin.Context`（DB, Guard, Encrypt, Audit, Notify, SendMail, OnEmail, DNS, cache, geo, webhooks, job queue…），且支持向下兼容演进。
- **路由自动门禁**（未启用时响应 404），**插件间通过 Service Registry 通信**（无直接 Cross-Import），**模型自动 AutoMigrate**。
- **无须修改代码即可组合自定义二进制** —— xcaddy 式驱动：
  ```bash
  OCTARQ_PLUGINS='[{"go":"github.com/you/octarq-plugin-foo","npm":"@you/octarq-plugin-foo"}]' make plugin-build
  ```

**几秒生成骨架** —— `octarq plugin new <name>` 直接生成可构建的 Go + web 插件骨架作为起点。

**官方插件与起步模板** → [octarq-plugins](https://github.com/octarq-org/octarq-plugins)（Telegram、Webhook、agent-native 的 Mail Links demo，以及可复制的 `_template`） · **详细开发指南** → [插件编写指南](https://docs.octarq.org/writing-a-plugin/)

*Octarq 自己的商业 Pro 版也只是基于这同一套公开接口构建的另一组插件 —— 没有任何社区做不到的事情。*

---

## 配置指南

- **通过 Cloudflare Worker 接收邮件** —— 部署 [`deploy/cloudflare-email-worker.js`](deploy/cloudflare-email-worker.js)，配置 catch-all 路由，并开启 *Accept email*。
- **GeoIP 地理位置统计** —— 设置 `OCTARQ_MAXMIND_LICENSE_KEY`（免费），Octarq 会自动下载并热加载 GeoLite2。参阅 [`deploy/GEOIP.md`](deploy/GEOIP.md)。
- **Claude Desktop 配置 MCP** —— 在 `claude_desktop_config.json` 中指向 `octarq mcp`（配置见上）。
- **备份与恢复** —— `octarq backup` / `octarq restore` 备份数据库。请备份整个 `/data` 目录（数据库 **以及** 自动生成的 `octarq.secret` / `octarq-admin-password.txt` 密钥文件），并注意 Postgres 备份需要宿主机上的 `pg_dump`。参见 [备份与恢复](https://docs.octarq.org/backup-restore/)。

---

## 本地开发

```bash
OCTARQ_SECRET_KEY=dev OCTARQ_ADMIN_PASSWORD=dev go run .   # 后端 API :8080
make dev                                                  # Vite 前端开发服务 (代理 /api)
go test ./... -race
```

欢迎贡献代码 —— 参见 [CONTRIBUTING.md](CONTRIBUTING.md)。

---

## 开源协议

基于 [MIT 协议](LICENSE) 开源。

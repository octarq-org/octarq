# D-driftdocs —— 文档承诺 vs 代码现实

## 结论（最危险的三条，各一句）

1. `website/src/content/docs/core/mcp.md:46` 发明了不存在的 `OCTARQ_MCP_ORG_ID`，并承诺"每个租户起一个进程即隔离"——实际 `internal/mcp/mcp.go:36` 写死 `stdioOrgID uint = 1`，照做只会让每个进程都读 org 1 的数据（F1 + F4，样板问题）。
2. `README.md:49` / `README_ZH.md:49` 声称 `octarq mcp` "支持 stdio 及 SSE/stream"——该子命令只跑 stdio（`internal/mcp/mcp.go:153` `mcp.StdioTransport{}`），SSE/Streamable HTTP 是 HTTP 服务器上的端点（`internal/api/api.go:233-234`），不在 `octarq mcp` 里（F2）。
3. `README.md:73`、`README_ZH.md:73`、`quickstart.md:23`、`deploy.md:51` 声称 ~19MB 的 `scratch` 镜像——实际 `-trimpath -ldflags="-s -w"` 构建出的二进制 65MB，且发布镜像（`.github/workflows/release.yml:94` 用 `Dockerfile`）是 distroless 而非 scratch（F2）。

## F1 凭空发明

**1. `OCTARQ_MCP_ORG_ID`（样例问题，见结论 1）**
- 文档：`website/src/content/docs/core/mcp.md:46-47` "Tools are scoped to one operator via `OCTARQ_MCP_ORG_ID`; for multi-tenant, run one `octarq mcp` process per tenant."
- grep：`grep -rn 'OCTARQ_MCP_ORG_ID' --include='*.go' --include='Makefile' --include='*.yml' --include='*.sh' .` → **无匹配**（连同 vite/脚本/make 全范围）。
- 代码侧：`internal/mcp/mcp.go:36` `const stdioOrgID uint = 1`（org 写死，非环境变量）。
- 处置：改为真实机制一句话——`octarq mcp` 以 bootstrap org 运行；HTTP 传输（`/api/mcp/sse`、`/api/mcp/stream`）的 org 来自调用方 API token（`internal/api/mcp.go:49-53,62-66`）。

**2. `plugin.Info` 结构体的 `Version` 字段**
- 文档：`website/src/content/docs/architecture/plugin-context.md:28` 示例 `type Info struct { Title string; Version string; Requires []string }`。
- grep：`grep -n 'Version' plugin/plugin.go` → Info 结构体内无 `Version`（`plugin/plugin.go:836-879` 实际字段：Title、Description、Icon、Category、Tags、Group、Core、EnabledByDefault、Requires）。
- 代码侧：`plugin/plugin.go:872-875` `Requires []string` 存在；`Version` 字段无匹配。
- 处置：删掉 `Version` 行（F1 删句）。

**3. 博客文章的 `Plugin` 接口示例**
- 文档：`website/src/content/docs/guides/why-i-rewrote-a-saas-stack-as-a-go-plugin-framework.md:33-37` 声称接口含 `Init(ctx context.Context, pctx *plugin.Context) error` 与 `Mount(router *http.ServeMux, pctx *plugin.Context) error`。
- grep：`grep -rn 'func .*Init(' plugin/ app/ --include='*.go' | grep -v _test` 与 `grep -rn 'ServeMux.*pctx' plugin/` → 无匹配；真实接口在 `plugin/plugin.go:14-33`（`Name() string`、`Models() []any`、`Mount(mux Mux, ctx *Context)`）。
- 处置：替换为真实接口三方法。

## F2 描述与实现相反

**1. `octarq mcp` 的传输方式（见结论 2）**
- 文档：`README.md:49`、`README_ZH.md:49` "（`octarq mcp`，over stdio and SSE/stream）"。
- grep：`grep -rn 'StdioTransport\|NewNetworkedServerInstance' internal/mcp/*.go internal/api/*.go` → `internal/mcp/mcp.go:153` 只跑 `mcp.StdioTransport{}`；网络端点在 `internal/api/api.go:233-234`（`/api/mcp/sse`、`/api/mcp/stream`），由 `internal/api/mcp.go:48-70` 挂载，且不注册 `query_db_readonly`。
- 处置：改成事实并把端点路径写明。

**2. 自动生成密钥的文件名**
- 文档：`docs/PRE-LAUNCH.md:24` "（`.octarq_secret`）"。
- grep：`grep -rn 'octarq.secret\|\.octarq_secret' config/ docs/` → `config/secrets.go:21` `autoSecretFile = "octarq.secret"`；`docs/PRE-LAUNCH.md:24` 全仓唯一一处 `.octarq_secret`。
- 处置：改为 `octarq.secret`。

**3. 二进制 / 镜像体积与镜像基座（见结论 3）**
- 文档：`README.md:73`、`README_ZH.md:73`、`website/src/content/docs/quickstart.md:23`（"~19MB `scratch` image"）、`website/src/content/docs/deploy.md:51`（"（~19MB）"）。
- grep / 实测：`CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o TMPOUT . && du -h TMPOUT` → 65M；`.github/workflows/release.yml:94` 发布镜像用 `file: ./Dockerfile`，而 `Dockerfile:44` 是 `FROM gcr.io/distroless/static-debian12`；`scratch` 仅在 `deploy/Dockerfile.binary:14`（备选）。
- 处置：删掉体积数字，把"scratch 镜像"改为指向真实的最小镜像 `deploy/Dockerfile.binary`。

**4. GeoIP Country 版数据库的行为**
- 文档：`deploy/GEOIP.md:146-147` "The Country edition also works but yields no region/city."
- grep / 源码：`internal/geo/geo.go:149-179` 的 `Locate` 只调 `db.City(parsed)`，出错即 `return "", "", ""`——对 Country-only 数据库 `City()` 返回 `InvalidMethodError`（geoip2-golang v1.13.0 `reader.go:335-340` 的 `isCity&r.databaseType == 0` 分支），因此连 country 也拿不到，不是"只缺 region/city"。
- 处置：删除该句（只留"Only **GeoLite2-City** is needed."）。

**5. 博客的 docker run 镜像引用**
- 文档：`website/src/content/docs/guides/why-i-rewrote-a-saas-stack-as-a-go-plugin-framework.md:23` "`docker run ... octarq/octarq`"。
- grep：`grep -rn 'ghcr.io/octarq-org/octarq' README.md docker-compose.yml .github/workflows/release.yml` → 全仓规范引用为 `ghcr.io/octarq-org/octarq`；`octarq/octarq` 全仓无出处。
- 处置：改为 `ghcr.io/octarq-org/octarq:latest`。

**6. 博客的 `RegisterNotifier` 示例签名**
- 文档：`website/src/content/docs/guides/why-i-rewrote-a-saas-stack-as-a-go-plugin-framework.md:53` "`pctx.RegisterNotifier("slack", func(ctx context.Context, cfg string, msg notify.Message) error {...})`"。
- grep：`grep -n 'RegisterNotifier' plugin/plugin.go` → `plugin/plugin.go:236` `RegisterNotifier func(typ string, send func(ctx context.Context, cfgJSON, text string) error)`。
- 处置：示例签名改为 `func(ctx context.Context, cfgJSON, text string) error`。

**7. 博客的 OpenAPI "自动生成" 表述**
- 文档：`website/src/content/docs/guides/why-i-rewrote-a-saas-stack-as-a-go-plugin-framework.md:77` "is automatically documented and browsable via `/openapi.json`..."。
- grep：`grep -rn '"/openapi.json"' app/ internal/ --include='*.go' | grep -v _test` → 服务端无该路由；spec 由 `openapi/openapi.go`（手写 map，`octarq openapi` 打印到 stdout，`cmd/openapi-gen/main.go`），我实测 `go run cmd/openapi-gen/main.go` 的输出与 `website/public/openapi.json` 逐字节一致。"自动"不成立。
- 处置：删"automatically"（文档与交互式 API 参考页仍然真实存在）。

**8. 博客对 SDK 的指认**
- 文档：`website/src/content/docs/guides/why-i-rewrote-a-saas-stack-as-a-go-plugin-framework.md:78` "Client SDKs (like `@octarq/plugin-sdk`) stay 100% in sync with backend Go definitions."
- grep：`packages/plugin-sdk/package.json` 是 UI 组件 SDK（依赖 `@base-ui/react`、`class-variance-authority` 等，无生成器）；真正的 OpenAPI 生成客户端在 octarq-pro 的 `packages/api-client`（orval，`orval.config.ts`）。
- 处置：改为指认 `@octarq-org/api-client`，删掉"100%"。

**9. SDK 版本示例**
- 文档：`website/src/content/docs/guides/publishing.md:52`、`docs/PUBLISHING.md:78` 示例 "`"version": "0.8.0"`"。
- grep：`grep -n '"version"' packages/plugin-sdk/package.json` → 实际 `"version": "0.10.0"`。
- 处置：两处示例版本更新为 `0.10.0`。

## F3 已删功能残留

**1. `VITE_OCTARQ_PLUGINS` 开关**
- 文档：`docs/PLUGIN-ARCHITECTURE.md:197` "Build-time injection seam (`#octarq-plugins` / `VITE_OCTARQ_PLUGINS`)"。
- grep：`grep -rn 'VITE_OCTARQ_PLUGINS' web/plugins-manifest*.mjs web/vite.config.ts` → 无消费；`web/plugins-manifest-core.mjs:22-32` 的优先级只读 `OCTARQ_PLUGINS` › `OCTARQ_PLUGINS_MANIFEST` › `octarq.plugins.json`；只有 `web/src/vite-env.d.ts:5,8` 残留类型声明，再无代码读取。`overview.md:67` 已自我标明该开关被替换。
- 处置：从该行删除 `/ VITE_OCTARQ_PLUGINS`。

## F4 承诺了不保证的安全性质

**1. MCP 的"租户隔离"（与 F1-1 同句，样板问题）**
- 文档：`website/src/content/docs/core/mcp.md:46-47` "Tools are scoped to one operator via `OCTARQ_MCP_ORG_ID`; for multi-tenant, run one `octarq mcp` process per tenant."——"为多租户各起一个进程即隔离"是承诺了实现不保证的隔离性质：`internal/mcp/mcp.go:36` 写死 `stdioOrgID = 1`，每个 `octarq mcp` 进程读的都是同一 org 1 的数据，按文档操作的运维者会得到"每个进程都读 org 1"的结果。
- 处置：改为事实（org 作用域的真实来源）。

**2. GeoIP "Country 版也能用"的保证（同 F2-4）**
- 承诺"yields no region/city"即暗示 country 数据可用，实现里 `db.City()` 对 Country DB 直接报错，连 country 都不返回。已按 F2-4 删除该句。

## 核查通过的（说明你查了什么范围）

范围：`README.md`、`README_ZH.md`、`docs/*.md`、`website/src/content/docs/**`、`deploy/*.md`、`CONTRIBUTING.md` 中每一处描述可配置项 / 能力 / 行为 / 默认值 / 路径 / 安全性质的句子，逐一在代码（`.go`、Makefile、compose、vite 配置、脚本、Dockerfile、CI workflows）grep 出消费方。下表均通过：

| 类别 | 核查内容 | 证据（代码侧） |
| --- | --- | --- |
| 环境变量 | `OCTARQ_SECRET_KEY`（≥16 字节，provisioned 时硬失败）| `config/config.go:149-193`、`MinSecretKeyLen=16` |
| 环境变量 | `OCTARQ_ADMIN_USER`（默认 admin）、`OCTARQ_ADMIN_PASSWORD` | `config/config.go:153-154` |
| 环境变量 | `OCTARQ_ALLOW_PRIVATE_WEBHOOKS` / `_SMTP`（默认 false）、`OCTARQ_TRUST_PROXY`（默认 false，控制 X-Forwarded-*）| `config/config.go:156-160` |
| 环境变量 | `OCTARQ_DB_DRIVER`（sqlite/postgres）、`OCTARQ_DB_DSN`（默认 octarq.db）| `config/config.go:150-151,167-168` |
| 环境变量 | `OCTARQ_LISTEN`（默认 `:8080`）| `config/config.go:149`；`Dockerfile:50` EXPOSE 8080、compose 端口 `:8080` 映射一致 |
| 环境变量 | `OCTARQ_ENDPOINT` 是 Worker 变量非服务端 | `deploy/cloudflare-email-worker.js:37` 消费；configuration.md 已注明"设在 Worker 上" ✓ |
| 环境变量 | `OCTARQ_MAXMIND_LICENSE_KEY`、`OCTARQ_GEOIP_DB`（优先于自动下载）| `internal/geo/geo.go:50-58,99-122`，`download.go`（sha256 校验、60 天过期提示、内存加载）|
| 环境变量 | LLM 组：`PROVIDER`（claude/openai/gemini/mistral/cohere/ollama 全注册）、`API_KEY`（回退 ANTHROPIC_API_KEY）、默认模型 `claude-opus-4-8`/`claude-haiku-4-5` | `llmprovider/provider.go:38-39,136-175`，`langchain.go:42-46` |
| 环境变量 | `OCTARQ_PLUGINS` › `OCTARQ_PLUGINS_MANIFEST` › `web/octarq.plugins.json` 优先级；格式 `{ "plugins": [...] }` | `web/plugins-manifest-core.mjs:13-32` |
| 环境变量 | `OCTARQ_WEBEMBED_OUT`（build 期 outDir）| `web/vite.config.ts:101` |
| 环境变量 | `OCTARQ_PORT`（工具链，仅 Makefile/vite 读取，非服务端变量——真变量）| `Makefile:49-53`、`web/vite.config.ts:7-8` |
| 环境变量 | `OCTARQ_SHARED_HOSTS` 及"注册首个域名后回退关闭"行为 | `origin/origin.go:29-42,234` |
| 环境变量 | `OCTARQ_CORS_ORIGINS`（仅启动 bootstrap 回退）+ 凭证从不跨域 | `config/config.go:73-79,165`；`internal/api/settings.go:164` |
| CLI | `octarq`（默认 HTTP 服务）、`octarq mcp`、`octarq plugin new`、`octarq openapi`、`octarq backup`、`octarq restore`、`--version` 全部有 dispatch | `main.go:40-78`、`cli_plugin.go:17-21`、`cmd_backup.go:16,51` |
| 限流 | auth/API/redirect 默认 60/600/6000 每分钟每 IP | `internal/server/middleware.go:339-341`（另有 login 5/15min、register 5/h、send 100/h 等 `api.go:105-110`）|
| 注册 | 开放注册默认开、邮件验证默认开 | `internal/api/settings.go:52,61,174-190` |
| 指标 | `/metrics` 无 token 时仅回环 | `internal/server/middleware.go:254,570` |
| 留存 | 数据留存 0 = 不删除 | `internal/api/settings.go:50`（"0 = disabled"）|
| 备份 | Settings → Instance 全库下载真实存在；无内置备份调度器 | `web/src/api.ts:500-501`（`/api/admin/backup`）；全仓 grep 备份侧无 scheduler |
| 部署 | `/data` 卷、`OCTARQ_DB_DSN=/data/octarq.db`、65532 用户、`/octarq` 入口；compose 卷名 `octarq-data` | `Dockerfile:44-54`、`docker-compose.yml:5-18,28-29` |
| 部署 | 首次启动生成密钥与密码并落盘到 DB 旁、日志打印一次 | `config/secrets.go:30-130`（`octarq.secret`、`octarq-admin-password.txt`）|
| MCP | 工具清单 `list_links/list_mailboxes/list_emails/list_domains/export_data/query_db_readonly` | `plugins/links/mcp.go:41`、`plugins/mail/mcp.go:46,51`、`plugins/dns/mcp.go:30`、`internal/mcp/mcp.go:166-181` |
| MCP | 守卫：仅 `SELECT`/`WITH` 前缀、读写/`PRAGMA`/`ATTACH` 关键字拒绝、只读事务、行数上限 200、敏感列脱敏 | `plugin/sqlguard.go:30-98`、`internal/mcp/mcp.go:207-237` |
| 插件系统 | `Plugin`/`MenuProvider`/`Starter`/`MCPProvider`/`OpenAPIContributor`/`Describer` 接口；`Context.DB/Guard/Encrypt(AES-256-GCM)/Decrypt/Audit/Notify/SendMail/OnEmail/DNS/UserID/OrgID/GetSetWorkspaceSetting/Huma/RequireRole/LookupAs/Provide/HandleStatic/RegisterNotifier` 全存在 | `plugin/plugin.go:14-33,223-402,804-879`、`plugin/registry.go:96,113` |
| 插件系统 | 路由自动 404 门禁、Requires 缺失拒绝启动、表名撞车 preflight | `app/app.go:105-107,111`、`app/preflight.go:107-123` |
| 其他 | webembed `go:embed` 仪表盘；`oct_` 前缀 API token；SHA-256 哈希 | `webembed/embed.go:13-14`、`internal/api/mcp.go:23`、`internal/models/models.go:271-273` |
| 其他 | `configuration.md` Hostnames 一节：`/admin` 之外、注册为 short-link/mail 的 host 不提供登录面 | `internal/server/server.go:111-112,178` |

备注（超出文档范围、未修改）：`app/preflight.go:120` 的错误文案提示 `build tags octarq_no%s`，全仓 `grep -rn 'go:build' --include='*.go'` 无任何 build tag 存在——这是 Go 源码内的注释性残留，属另一条线（代码注释）的排查面，本文档未触碰。
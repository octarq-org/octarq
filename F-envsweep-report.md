# F-envsweep Report — 环境变量真伪清扫 + 备份文档

工作目录: `.worktrees/octarq-envsweep`（分支 `fix/envsweep`）。本报告只覆盖本次改动，
所有路径为仓库相对格式。

## 方法与范围

- Go 侧消费方: `grep -rn '"OCTARQ_[A-Z_]*"' --include='*.go'`（排除测试文件）+
  `grep -rn 'os.Getenv|os.LookupEnv' --include='*.go'` 双重交叉。
- 工具链/部署侧: `Makefile`、`docker-compose.yml`、`web/vite.config.ts`、
  `web/plugins-manifest-core.mjs`、`deploy/`、`Dockerfile*`、`.air.toml` 逐一核对。
- 全仓去重盘点: `grep -rhoE 'OCTARQ_[A-Z0-9_]+'`（排除 node_modules / dist / .git）。
  每个出现的令牌都已归类，见下表。

## 四类归类总表

### A 类 · Go 二进制读取（全部为真变量，保留）

| 变量 | 消费方 file:line | 动作 |
| --- | --- | --- |
| `OCTARQ_LISTEN` | `config/config.go:149` | 保留（已在 .env.example:7，配置文档已补） |
| `OCTARQ_DB_DRIVER` | `config/config.go:150`；`internal/geo/download.go:66` | 保留 |
| `OCTARQ_DB_DSN` | `config/config.go:151`；`internal/geo/download.go:70` | 保留 |
| `OCTARQ_SECRET_KEY` | `config/config.go:152` | 保留 |
| `OCTARQ_ADMIN_USER` | `config/config.go:153` | 保留 |
| `OCTARQ_ADMIN_PASSWORD` | `config/config.go:154` | 保留 |
| `OCTARQ_TRUST_PROXY` | `config/config.go:156` | 保留 |
| `OCTARQ_ALLOW_PRIVATE_WEBHOOKS` | `config/config.go:158` | 保留 |
| `OCTARQ_ALLOW_PRIVATE_SMTP` | `config/config.go:160` | 保留 |
| `OCTARQ_GEOIP_DB` | `config/config.go:162` | 保留 |
| `OCTARQ_REDIS_URL` | `config/config.go:163` | 保留（已补进 configuration.md 表格） |
| `OCTARQ_CORS_ORIGINS` | `config/config.go:165` | 保留 |
| `OCTARQ_SHARED_HOSTS` | `origin/origin.go:244` | 保留（已补进 configuration.md 表格） |
| `OCTARQ_BASE_DOMAIN` | `internal/models/base_domain.go:22,39` | 保留（已补进 configuration.md 表格） |
| `OCTARQ_MAXMIND_LICENSE_KEY` | `internal/geo/geo.go:58` | 保留 |
| `OCTARQ_LLM_API_KEY` | `llmprovider/provider.go:164` | 保留 |
| `OCTARQ_LLM_PROVIDER` | `llmprovider/provider.go:169` | 保留 |
| `OCTARQ_LLM_BASE_URL` | `llmprovider/provider.go:171` | 保留 |
| `OCTARQ_LLM_MODEL` | `llmprovider/provider.go:172` | 保留 |
| `OCTARQ_LLM_CHEAP_MODEL` | `llmprovider/provider.go:173` | 保留 |

### B 类 · 工具链 / 构建 / 部署脚本读取（真变量，不进二进制，保留）

| 变量 | 消费方 file:line | 动作 |
| --- | --- | --- |
| `OCTARQ_PORT` | `Makefile:49-53`；`docker-compose.yml:7`；`web/vite.config.ts:9` | 保留（见下方「PORT 澄清」）；已补进 configuration.md 表格并写明语义 |
| `OCTARQ_PLUGINS` | `cmd/octarq-build/main.go:59`（构建工具，非运行二进制）；`Makefile:60,66,68`；`web/plugins-manifest-core.mjs:18` | 保留（构建期变量，README/PLUGINS.md 已文档化） |
| `OCTARQ_PLUGINS_MANIFEST` | `web/plugins-manifest-core.mjs:23-24` | 保留（构建期变量，已文档化） |
| `OCTARQ_WEBEMBED_OUT` | `web/vite.config.ts:101` | 保留（构建期变量，已文档化） |
| `OCTARQ_DEV_ROOTS` | `web/vite.config.ts:18` | 保留（仅 vite 开发期，不进 .env.example，属开发工具） |
| `OCTARQ_DEV_ALIASES` | `web/vite.config.ts:27` | 保留（同上） |
| `OCTARQ_ENDPOINT` | `deploy/cloudflare-email-worker.js:37` | 保留（Cloudflare Worker 的环境变量，非 octarq 二进制；configuration.md/mailboxes.md 已文档化） |
| `OCTARQ_E2E_PORT` / `OCTARQ_E2E_TMPDIR` | `web/playwright.config.ts:11,17,19,24` | 保留（e2e 测试工具链，不进 .env.example） |
| `OCTARQ_PRO_DIR` | `web/scripts/i18n-audit.mjs:16` | 保留（dev 脚本读取，非运行配置） |

注: `OCTARQ_PLUGINS` 的 `grep '"OCTARQ_[A-Z_]*"' --include='*.go'` 会命中
`cmd/octarq-build/main.go:59`（非测试 Go 文件），按字面规则似为 A 类；但
`cmd/octarq-build` 是 `make plugin-build` 驱动的构建期代码生成工具，从不链接进运行
二进制，其真实消费方是构建链（Makefile + vite），故按规格 B 类语义（"真变量，只是不
进二进制"）归类，并在此说明判定依据。

### C 类 · 无任何消费方（假变量）

| 变量 | 出现处 | 核对范围 | 动作 |
| --- | --- | --- | --- |
| `OCTARQ_MCP_ORG_ID` | `website/src/content/docs/core/mcp.md:46`（原） | 全仓 grep（排除 node_modules/dist/.git）仅此一处；Go 侧 `internal/mcp/mcp.go:33-36` 显示 stdio MCP 固定运行于 bootstrap org（`stdioOrgID = 1`），无 org 选择环境变量 | **删除**：已改写该文档句为真实机制描述（见改动文件） |
| `OCTARQ_APP_NAME` | `plugins/help/docs/quickstart.mdx:19`、`quickstart.zh.mdx:19` | Go/配置/文档全仓 grep 无读取方；默认应用名来自 ldflags（`config/config.go:22`）与仪表盘 `app_name` 设置，非环境变量 | 假变量，但位于 `plugins/`（他人地盘，本任务不碰），**报告待 plugins agent 修正** |
| `OCTARQ_WEBHOOK_SECRET` | `plugins/help/docs/webhooks.mdx:35`、`webhooks.zh.mdx:35` | Go/配置全仓 grep 无读取方；webhook 签名密钥来自每个 endpoint 在仪表盘配置/自动生成的 Signing Secret（`internal/eventbus/eventbus.go:41-44,137-138`），非环境变量 | 假变量，位于 `plugins/`（他人地盘），**报告待 plugins agent 修正** |

非环境变量字符串（非 OCTARQ 运行配置，仅作盘点说明，无动作）:
`OCTARQ_PRO_LICENSE`（`web/scripts/i18n-audit.mjs:258`，i18n 审计白名单字面量）、
`VITE_OCTARQ_PLUGINS`（历史 Vite 变量，已被 manifest 机制取代，文档
`website/src/content/docs/architecture/overview.md:67` 已如实说明）。

### D 类 · 代码读取但此前文档缺失（均已补）

反向扫描结论: 二进制读取的 20 个 A 类变量**全部**已存在于 `.env.example`（逐项比对
`.env.example:2-76`，无缺失），因此严格按规格定义无 D 类变量需补进 `.env.example`。
但 website 的 `configuration.md` 表格此前缺了 5 个真实变量，本次补齐（属文档补全，
不是新增变量）:

| 变量 | 消费方 | 补齐位置 |
| --- | --- | --- |
| `OCTARQ_LISTEN` | `config/config.go:149` | `website/src/content/docs/configuration.md` Core 表 |
| `OCTARQ_PORT` | `Makefile:49-53`,`docker-compose.yml:7`,`web/vite.config.ts:9` | 同上（含语义澄清） |
| `OCTARQ_REDIS_URL` | `config/config.go:163` | `website/src/content/docs/configuration.md` Database 表 |
| `OCTARQ_SHARED_HOSTS` | `origin/origin.go:244` | `website/src/content/docs/configuration.md` Hostnames 小节 |
| `OCTARQ_BASE_DOMAIN` | `internal/models/base_domain.go:22,39` | 同上 |

## OCTARQ_PORT 澄清（已知结论，予以采信并落实）

`OCTARQ_PORT` 为 **B 类真变量**，消费方 `Makefile:49-53`、`docker-compose.yml:7`、
`web/vite.config.ts:9`。Go 二进制确实不读它（二进制监听端口用 `OCTARQ_LISTEN`）。
为让"宿主端口/开发端口，不是二进制监听端口"无法被误解:
- `.env.example:8-10` 注释已写明 "Docker compose only … use OCTARQ_LISTEN"（已有，保留）。
- 本次在 `configuration.md` 新增一行表格，明确指出 "Not read by the binary; to change
  the binary's own listen port use OCTARQ_LISTEN"。

## 历史遗留（只报告，不改测试）

`.env.example:27-33` 声明的三个已删变量 `OCTARQ_BASE_URL` /
`OCTARQ_ADMIN_HOST` / `OCTARQ_SECURE_COOKIES` 在 `config/config_test.go:171-173`
仍有 `t.Setenv` 死引用（并有多处 Go 注释提及它们被重构删除）。按规格只报告不修改。

文档侧处理: `.env.example:27-33` 已有完整的 "There is no base-URL, admin-host or
secure-cookie variable" 解释注释；各 md 文档无任何残留引用（全仓 grep 仅在 Go 注释与
测试中出现）。**文档侧无需改动**，保持现状即可。测试侧改动归 config agent。

## 备份/恢复文档（prelaunch M4）

新增 `website/src/content/docs/backup-restore.md`（sidebar group "Start"，order 6），
覆盖规格要求的三件事:

1. **只备份 DB 不够** —— 说明零配置模式下 `octarq.secret` 与 `octarq-admin-password.txt`
   （`config/secrets.go:21-23`，位于 sqlite 库旁；Docker 镜像内为 `/data/`）是解密存量
   凭据（TOTP、插件凭据）与 admin 密码的唯一来源，要求**备份整个 `/data`**。
2. **postgres 依赖宿主机 `pg_dump`** —— 引用 `internal/db/backup.go:168`，明确
   scratch/distroless 镜像不含 `pg_dump`/`pg_restore`/`psql`，postgres 备份恢复
   **不能在容器内跑**，需在装有 `postgresql-client` 的宿主机/sidecar 执行。
3. **sqlite 一致性快照 + restore 安全网** —— 说明 sqlite 走 `VACUUM INTO` 在线非锁定
   快照（`internal/db/backup.go:143`）；restore 前自动先备份当前库
   （`octarq-backup-before-restore-*`）并要求交互确认（`cmd_backup.go:74-97`）。

可链性: `website/src/content/docs/deploy.md` §5 链接 `/backup-restore/`；
`docs/PRE-LAUNCH.md` §3 增加指向该文档的相对链接；`README.md` 与 `README_ZH.md`
Setup/配置指南各加一条 Backup & Restore 条目。

## 改动文件清单

- `website/src/content/docs/core/mcp.md` — 删除 `OCTARQ_MCP_ORG_ID` 假变量引用，改写为真实机制。
- `website/src/content/docs/configuration.md` — 补齐 `OCTARQ_LISTEN`/`OCTARQ_PORT`/`OCTARQ_REDIS_URL`/`OCTARQ_SHARED_HOSTS`/`OCTARQ_BASE_DOMAIN` 表格行。
- `website/src/content/docs/backup-restore.md` — 新增（Task 2）。
- `website/src/content/docs/deploy.md` — §5 链接备份文档。
- `docs/PRE-LAUNCH.md` — §3 链接备份文档。
- `README.md` / `README_ZH.md` — Setup/配置指南加备份条目。

`internal/`、`app/`、`web/`、`plugins/`、`main.go`、`config/` 未做任何修改。

## 验证

- `go build ./...` → exit 0。
- `gofmt -l .` → 无输出。
- `grep -rn 'OCTARQ_MCP_ORG_ID'`（排除 node_modules/dist/.git）→ 无匹配（已清除）。
- `.env.example` 未改动（回归确认: 其中 20 个 A 类 + 1 个 B 类变量，无假变量）。

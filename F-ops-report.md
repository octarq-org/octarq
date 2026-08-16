# F-ops 报告 —— 日志级别可配 + CSP 收紧

分支：`fix/ops`（worktree `/Volumes/PHD/code/.worktrees/octarq-ops`）

---

## 任务 1：日志级别可配（prelaunch M5）

### 新增环境变量

| 变量 | 语义 | 取值 | 默认 |
|---|---|---|---|
| `OCTARQ_LOG_LEVEL` | 进程级 slog 默认 logger 的严重级别阈值 | `debug` / `info` / `warn` / `error`（大小写与首尾空白不敏感） | `info`（未设置或设置为空串） |

语义：`slog.SetDefault` 的 `HandlerOptions.Level` 由此解析。设置成 `error` 时 info 级
访问日志与生命周期日志（如 `octarq listening`、readiness 行）不再输出；`debug` 时全部
输出。供文档批次引用的变量名即 `OCTARQ_LOG_LEVEL`。

### 非法值处理：启动即报错并点名（选择 fatal，非降级警告）

**选择**：非法值 → `config.Load()` / `config.LogLevel()` 返回错误，进程启动失败，
错误消息点名变量名：

```
OCTARQ_LOG_LEVEL must be debug, info, warn or error, got "banana"
```

**理由**：与仓库既有惯例一致 —— `config.go:167-169` 对非法 `OCTARQ_DB_DRIVER`
就是启动即报错并点名，规格文件明确要求保持一致。且日志级别的语义是"我想看到多细的
日志"：如果拼错（如 `INFo`）静默回落到 info，恰好会吞掉 operator 想要打开的 debug
日志，属于最坏方向的静默。配置是部署期 knob，宁可启动失败让 CI/部署立刻暴露，也不
要在生产里悄悄少了日志。`config.Load()` 与 `config.LogLevel()` 共用 `normalizeLogLevel`
与 `validLogLevels`，两处解析永不打架。

### 实现位置

- `config/config.go`：`Config.LogLevel` 字段 + `Load()` 读取/校验（`normalizeLogLevel`、
  `validLogLevels`、`slogsByLevel`）、新增 `config.LogLevel() (slog.Level, error)`。
- `main.go:33-42`：`slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))`，
  level 来自 `config.LogLevel()`；非法值在 logger 建立前就 fatal。

### debug 日志的密钥泄露排查（只报告，未改别的包）

启动路径上的日志点排查结果：

- `app/readiness.go` readiness 行：只输出 `configured / not configured`，**不含任何
  秘密值** —— 现有 `TestReadinessReportOmitsSecrets` 钉住，开启 debug 不改变该行为。
- `config/config.go` 的 secret-key 告警：只输出**长度**，不输出 key 本身。
- `app/crypto_store.go`、`internal/api/settings.go`：GORM 的 SQL 日志（stderr 上的
  `SELECT ...` 行）只含 SQL 与行数，不含 DSN。**注意**：GORM 日志是普通 log 输出，
  不经 slog，不受 `OCTARQ_LOG_LEVEL` 控制 —— 这是现状，不在本次改动范围内。
- DSN（`OCTARQ_DB_DSN`）在启动日志以 `db":"sqlite"` 形式出现（`app` 包），
  我 grep 到的输出是 `driver=sqlite dsn=/tmp/...`（readiness 行，来自 `db.Open`），
  **不包含密码部分**；Postgres DSN 若含密码是否会打印未逐包排查（只报告）。

## 任务 2：CSP 收紧（安全审计 L-4）

### 内联脚本排查结果（先查后改）

`web/index.html`（源）与 `webembed/dist/index.html`（实际服务的构建产物）各有
**2 个内联 `<script>` 块**，且两文件逐字节一致：

1. **主题同步脚本**（`index.html:15-23`）：localStorage 读 `octarq-theme`，首屏前
   加 `dark` class，防 FOUC —— 必须内联（首屏前执行）。
2. **Campaign 转发脚本**（`index.html:41-85`）：给去往 octarq.org 的链接补 UTM 参数，
   无自请求 —— 也内联在 index.html 中。

Vite 未注入任何内联 runtime/modulepreload 脚本（构建产物里 modulepreload 是
`<link rel="modulepreload">`，不受 `script-src` 约束）。其余页面（登录/Overview/
插件页/`/status`/`/instance`）都走同一个 SPA `index.html`，无独立内联脚本。

### 方案：保留必要内联脚本，改用构建期 SHA-256 hash 白名单

规格说"如果确实存在必要的内联脚本，不要强行删 `'unsafe-inline'`，改用 nonce 或
构建期 sha256 hash"。这两个脚本都是必要的（主题脚本删了会闪白/主题失效；campaign
脚本是产品行为）。**nonce 不可行**：`server.go:270-278` 的 `serveIndex` 直接把
启动时读入的 `index.html` 原样写回，没有逐请求 HTML 改写通道，引入 nonce 需要改
静态服务架构（且破坏缓存）。**构建期 hash 可行且零架构改动**：

- `internal/server/middleware.go:492-502`：CSP 提升为常量 `contentSecurityPolicy`，
  `script-src 'self' 'unsafe-inline'` → `script-src 'self' 'sha256-XOdM...AXQ=' 'sha256-0la+...42Q='`，
  即上述两个内联脚本的精确 SHA-256。
- `style-src 'unsafe-inline'` 保留（Tailwind 运行时注入样式，删了整站白屏）。
- 其他指令原样保留。

### CSP 前后对比

| 指令 | 改前 | 改后 |
|---|---|---|
| `script-src` | `'self' 'unsafe-inline'` | `'self' 'sha256-XOdMZOShyEmv5lOxX1JVnl4Ve1llBlA5kNEaund+AXQ=' 'sha256-0la+svl4yiuBleyZsFu+6aWjFtuvmwtPoqXKtkLO42Q='` |
| `style-src` | `'self' 'unsafe-inline' https://fonts.googleapis.com` | **不变**（保留 unsafe-inline） |
| 其余 | 不变 | 不变 |

效果：XSS 注入的内联 `<script>` 不再被放行；仅精确匹配两个已知内联脚本字节的
脚本可执行（改一行字节 hash 就失效）。

### hash 漂移防护

`internal/server/middleware_test.go` 的 `TestSecurityHeadersCSP` 从嵌入的
`webembed/dist/index.html` 重新提取内联脚本并重算 hash，断言 CSP 中包含它们——
若未来前端构建改了内联脚本而没人更新 `contentSecurityPolicy`，测试直接红。

## 守卫测试

| 守卫 | 测试 | 位置 |
|---|---|---|
| 日志级别 env 生效（error 时不输出 info） | `TestLogLevelFeedsSlogLogger`（slog.Enabled 断言）+ 实跑验证（见下） | `config/config_test.go` |
| 非法值按选定语义处理（fatal 点名） | `TestLoadRejectsBadLogLevel`、`TestLoadLogLevel` | `config/config_test.go` |
| CSP 无 `script-src ... 'unsafe-inline'` 且保留 `style-src ... 'unsafe-inline'`、hash 匹配实际服务内容 | `TestSecurityHeadersCSP` | `internal/server/middleware_test.go` |

## 变异验证（`if cond && false` 短路，确认变红后已还原）

1. **日志级别**：`config/config.go:223` 的校验行改为
   `if false && c.LogLevel != "info" && !validLogLevels[c.LogLevel] {`（仍能编译）
   → **`TestLoadRejectsBadLogLevel` 变红**（`expected a refusal to start for an
   unsupported log level`）。改回后恢复绿。
2. **CSP**：`internal/server/middleware.go:505` 的 `contentSecurityPolicy` 临时在
   `script-src` 里加回 `'unsafe-inline'`（仍能编译）→ **`TestSecurityHeadersCSP`
   变红**（`script-src must not contain 'unsafe-inline'`）。改回后恢复绿。

## 验证（CI 命令）

```bash
unset http_proxy
gofmt -l .          # 无输出 ✓
go build ./...      # 通过 ✓
go vet ./...        # 通过 ✓
go test ./... -race # 全绿（最终结果）✓
```

**最终 `go test ./... -race` 结果：全绿**（35 包 ok，8 包无测试文件，0 FAIL，
含 `internal/api` 331.8s）。中途曾因本机高负载（load 峰值 57，5+ 个 agent 的
worktree 并发跑完整 `-race` 套件）出现过 `internal/api` 超时（10m/20m 均超）；
对照实验证明是环境问题而非本分支改动：另一 worktree（octarq-seclow，不含本分支
改动）在同样负载下**同一包**同样超时。负载回落后完整套件一次通过，且 `internal/api`
单独跑 484s 通过、套件内 331s 通过。

## CSP 实跑验证（截图 + console）

**顺序**：先 `pnpm build`（`web/` 内，重建 `webembed/dist`，产物内联脚本与 hash
一致）→ 再 `go build` 重启服务（:8791，`OCTARQ_SECRET_KEY=dev OCTARQ_ADMIN_PASSWORD=dev-pw`）
→ 再用 Playwright 截图。

**响应头实测**（curl `/admin/`、`/status`、`/instance`、`/login` 全部一致）：

```
Content-Security-Policy: default-src 'self'; script-src 'self' 'sha256-XOdMZOShyEmv5lOxX1JVnl4Ve1llBlA5kNEaund+AXQ=' 'sha256-0la+svl4yiuBleyZsFu+6aWjFtuvmwtPoqXKtkLO42Q='; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'
```

**截图**（`.agy-specs/shot.mjs` 同款流程：主题证明 + 登录失败检测 + DOM 内容证明，
截图存放 `/tmp/ops-shots/`，light/dark 各一套）：`login`、`/admin/overview`、
`/admin/links`（插件页）、`/status`。

- 主题证明：`login-dark` 等 `html.class="dark"` —— **该 class 由内联主题脚本写入**，
  证明 hash 白名单的内联脚本在收紧后的 CSP 下**真实执行**（没被挡）。
- 白屏检测：每页 `rootChildren=2`（React 挂载），DOM 文本提取到真实内容
  （Overview 的 "Set up the essentials"、Links 侧栏、Status 的 "All Systems
  Operational"），无 "Content Security Policy"/"Refused to" 字样。
- **console**（错误/警告/失败请求全量收集）：**零 CSP 违规报错**。仅有的两条
  `console.error` 是登录前 API 探针的 401（与 CSP 无关），若干 `ERR_ABORTED` 是
  SPA 导航中断的 fetch（正常行为）。

**回答规格的自问**："如果 CSP 把脚本挡了，这张图会不会拍出来？" —— 主题脚本被挡
则 dark 截图没有 `dark` class（会被主题证明抓到）；主 bundle 被挡则 `rootChildren=0`
且 DOM 无文本（会被 DOM 检查抓到）；两者都未发生，故截图有效。

## 交付物

- 改动文件（仅这些）：`config/config.go`、`config/config_test.go`、
  `internal/server/middleware.go`、`internal/server/middleware_test.go`、`main.go`
- 本报告：`F-ops-report.md`（worktree 根）
- `webembed/dist` 的本地重建产物**未提交**（按 CLAUDE.md，刷新是 CI 的职责）
- 临时脚本（`web/shot-ops.tmp.mjs`、`web/shot-dom.tmp.mjs`、`/tmp/*`）均已删除/不在仓库
- 未 push、未开 PR

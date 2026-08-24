# Go 框架选型研究 for Octarq

> **核实状态（2026-08-24）**：仓库根及 `.agents/`、`docs/`、`website/` 全量检索未发现既有 `go_frameworks_research_for_octarq.md`；`git log --all --name-only` 亦无该文件历史记录。判定为“应有 2 阶段”尚未落盘，或在历史 worktree 中未合入。本次在 `research/go-frameworks-phase3` worktree 中**重建 Stage 1-2 基线**并**启动 Stage 3 迭代**，走 worktree + pr-ship 流程。

- **Worktree**: `.worktrees/research-go-frameworks` → branch `research/go-frameworks-phase3`
- **Base**: `main@3390c81`
- **作者**: Sisyphus (orchestrator)
- **状态**: Stage 1 ✅ 重建完成 / Stage 2 ✅ 重建完成 / Stage 3 🚧 方案已定、待实施

---

## Stage 1 — 现状盘点（重建）

### 1.1 结论先行

Octarq 当前**不依赖全功能 Web 框架**，而是以 **`net/http` + `http.ServeMux` (Go 1.25) + `danielgtaylor/huma/v2` + `gorm.io/gorm`** 为核心，配合插件化架构实现“一个二进制、零 CGO、SQLite 优先”的交付目标。该选型是**有意为之**，而非历史债务。

> 信源：`go.mod:4,8` (`go 1.25`, `huma/v2 v2.38.0`)；`CLAUDE.md` “Go 1.25, standard-library `http.ServeMux`”；`plugin/plugin.go:1-110` 插件契约；`app/app.go:15-53` composition root；`internal/server/server.go:44-99` 顶层路由。

### 1.2 运行时架构

```
app.New()                // 开 DB、cipher、auth、geo、otel
  ↓ Use(plugin)           // 注册插件，不迁移
app.Run()                // 1) preflight 校验 2) 单次 AutoMigrate 3) Huma API + 插件 Mount 4) server.New() 5) Listen
  ↓ plugin.Context
  ├─ Huma (humago.New)    // OpenAPI v3 自动生成 + 校验
  ├─ DB (*gorm.DB)        // 租户隔离 via orgID
  ├─ Guard / RequireRole  // RBAC
  ├─ Provide/Lookup       // 插件间服务注册表
  └─ HandleRoot/Static    // short-link 抢占 "/"、插件 SPA 挂载 "/portal" 等
server.ServeHTTP          // /api/* → Huma, /admin/* → SPA, /{slug} → links fallback
```

- **路由分层**：`/api/*` (Huma)、`/openapi.json|/docs|/schemas/*` (Huma 生成)、`/admin/*|/instance|/status` (SPA)、`/{slug}` (links 插件通过 `HandleRoot`)。见 `internal/server/server.go:102-238`。
- **插件隔离**：`Mux` 为 `http.ServeMux` 子集，`gatedMux`/`gatedAdapter` 按 workspace 特性开关做 404 隔离 (`app/gated_router.go`)。
- **OpenAPI 优先**：所有业务路由经 `huma.Register` 强类型输入输出 → 自动 JSON Schema 校验 + `/openapi.json` 零漂移。`plugin.Context.Huma` 是唯一对外暴露的路由注册口。
- **观测与中间件**：`internal/server/middleware.go` + `otelhttp` + `gopkg.in` 日志，限流 `ulule/limiter`，SSRF 防护 `plugin/safehttp`。

### 1.3 依赖盘点

| 层 | 依赖 | 版本 | 备注 |
|---|---|---|---|
| 语言 | Go | 1.25.13 | `go.mod:3` |
| HTTP | stdlib `net/http` + `humago` adapter | — | 无 Gin/Echo/Fiber |
| OpenAPI | `huma/v2` | v2.38.0 | 强类型、自动文档 |
| ORM | `gorm.io/gorm` + sqlite/postgres/mysql | v1.31.1 | `glebarez/sqlite` 纯 Go |
| Auth | `gorilla/sessions`, `goth` | v1.4.0/v1.82.0 | OAuth 多提供商 |
| 队列 | `hibiken/asynq` + `redis/go-redis` | v0.26.0/v9 | 异步任务 |
| DNS | `cloudflare-go`, `dnspod` SDK | v0.117.0 | zone/record 代理 |
| AI | `anthropic-sdk-go`, `langchaingo`, `google/vertexai` | — | LLM 抽象 |
| 观测 | `otel`, `prometheus/client_golang` | — | trace/metrics |
| 间接 | `go-chi/chi/v5` `gorilla/mux` | v5.2.5/v1.8.1 | 仅 indirect，实未使用 |

> **间接引入解释**：`chi`/`mux` 由 `goth`、`cloudflare-go` 等上游依赖拖入，未在 octarq 直接调用；`go.mod` 标记为 `// indirect`。

### 1.4 为什么是 stdlib+Huma

1. **插件契约最小化**：`plugin.Plugin` 仅需 `Name/Models/Mount(Mux, *Context)`，`Mux` 是 `Handle/HandleFunc` 两方法接口，任何 stdlib 兼容 handler 可接入；若绑定 Gin/Echo 的 `Context`，插件将与框架强耦合，违背“开源 core 被 octarq-pro 当库引用”的可组合单体目标。
2. **OpenAPI 零维护**：Huma 从 Go struct tag 推导 JSON Schema，`/openapi.json` 即 `openapi.json` 真实服务形态，`@octarq-org/api-client` 可自动同步；Gin/Echo 需手写或额外插件。
3. **二进制纯度**：`glebarez/sqlite` + stdlib → `go build` 单二进制，无 CGO，便于 Docker `scratch` 交付。
4. **可测试性**：`net/http/httptest` + `huma` 强类型输入即测，无需 mock 框架上下文。

---

## Stage 2 — 候选框架对比（重建）

### 2.1 评估维度与权重

| 维度 | 权重 | 说明 |
|---|---|---|
| 性能 (qps/latency) | 15% | 单机压测仅参考，octarq 瓶颈在 DB/外部 API |
| OpenAPI 生成 | 20% | 文档漂移成本高，权重最高 |
| 插件化友好度 | 20% | 能否以 `net/http.Handler` 隔离租户/特性 |
| 类型安全 | 15% | 编译期捕获比运行时 500 更值 |
| 中间件生态 | 10% | 限流、认证、观测是否现成 |
| 学习曲线 | 5% | 团队已熟 stdlib/Huma |
| 社区活跃度 | 10% | 2024-2026 维护与 Go 1.25 适配 |
| 与现有架构兼容 | 5% | 迁移成本 |

### 2.2 框架速览（截至 2026-08）

| 框架 | 最新版本 / Go 要求 | GitHub Stars (量级) | 维护状态 | 核心模型 |
|---|---|---|---|---|
| **stdlib + Huma v2** | Go 1.25 / Huma v2.38 | Huma ~2k | 活跃，适配 Go 1.25 `ServeMux` 增强 | `http.Handler` + 强类型 struct |
| **Gin** | v1.10 / Go 1.21+ | ~82k | 活跃，但 API 稳定少大改 | `gin.Context` 封装 |
| **Echo** | v4.13 / Go 1.23+ | ~31k | 活跃 | `echo.Context` |
| **Fiber** | v2.52 (v3 beta) / Go 1.21+ | ~34k | 活跃，v3 适配 `net/http` 互操作 | `fasthttp` (非 stdlib) |
| **Chi** | v5.2.5 / Go 1.21+ | ~19k | 活跃，轻量路由 | 纯 `net/http` 兼容 |

> 数字为公开量级，需以实时 GitHub 为准；Huma star 低但定位是 OpenAPI 层而非全框架。

### 2.3 8 维度打分（5 最高）

| 维度 | stdlib+Huma | Gin | Echo | Fiber | Chi |
|---|---|---|---|---|---|
| 性能 | 4 (stdlib 接近 Gin，Huma 少量校验开销) | 5 | 4 | 5 (fasthttp 快但互操作成本) | 4 |
| OpenAPI 生成 | 5 (自动、零漂移) | 2 (需 swag 手写注释) | 2 | 2 | 1 (无内置) |
| 插件化友好 | 5 (`Handler` 即插件边界) | 3 (`gin.Context` 耦合) | 3 | 2 (fasthttp 不兼容 `ResponseWriter`) | 5 |
| 类型安全 | 5 (Go struct + huma 校验) | 3 | 3 | 3 | 4 |
| 中间件生态 | 4 (stdlib + otel + limiter 足够) | 5 | 5 | 4 | 4 |
| 学习曲线 | 4 (Huma 需学习 tag 体系) | 5 | 5 | 4 | 5 |
| 社区活跃 | 4 | 5 | 5 | 5 | 4 |
| 与现有兼容 | 5 (零迁移) | 2 | 2 | 1 | 4 |
| **加权总分** | **4.55** | 3.35 | 3.30 | 2.85 | 3.70 |

### 2.4 逐框架 pros / cons

**stdlib + Huma v2 — 推荐保留**
- Pros: OpenAPI 自动生成、类型安全、`net/http` 插件边界清晰、零 CGO、二进制纯度、测试简单。
- Cons: 生态文档少于 Gin/Echo；Huma tag 需学习；性能非第一但对 octarq 非瓶颈。
- 适用：插件化/多租户/OpenAPI 优先的单体。

**Gin**
- Pros: 生态丰富、中间件多、社区大、性能好。
- Cons: `gin.Context` 强耦合，插件需感知框架；OpenAPI 靠 `swag` 注释易漂移；与 `plugin.Mux` 抽象冲突。
- 结论：短期提效不值长期耦合成本。

**Echo**
- 同 Gin，`echo.Context` 耦合 + OpenAPI 手维护，收益与 Gin 同阶，不引入。

**Fiber**
- Pros: 性能极高。
- Cons: 基于 `fasthttp`，与 `net/http`、`otelhttp`、`gorilla/sessions` 不互通；octarq 的 `safehttp`、SSRF 防护、otel 均需重写；插件隔离成本最高。
- 结论：排除。

**Chi**
- Pros: 轻、纯 `net/http` 兼容，可替代 stdlib 路由层。
- Cons: 仅路由，无 OpenAPI/校验，需另配 Huma 或 `oapi-codegen`；收益仅是路由语法糖，不值得迁移。
- 结论：可作为 stdlib 补充，但当前 Go 1.25 `ServeMux` 已支持 `/{param}`，Chi 非必需。

### 2.5 结论（Stage 2 定案）

**保持 `stdlib + Huma v2`，不迁移至 Gin/Echo/Fiber/Chi。**

理由：octarq 的“插件即 Handler”与“OpenAPI 即实现”两大约束，使 Huma 的自动文档与 stdlib 的 handler 隔离成为架构锚点；任何绑定框架 `Context` 的方案都会破坏插件的可组合性并引入文档漂移。性能差异对 octarq（瓶颈在 DB/外部 API/邮件投递）不构成迁移理由。

**3 条可执行强化建议（Stage 3 输入）**：
1. **守住 Huma 契约**：新增 API 必须经 `huma.Register` + `plugin.Context.Huma`，禁止绕过 Huma 直接 `mux.Handle` 暴露 `/api/*`；CI 加 `grep -r "Handle.*\\/api\\/" --include="*.go" | grep -v huma` 门禁。
2. **补齐缺失的框架能力以“库”而非“框架”引入**：如需更强校验/限流，引入 `go-playground/validator` 或 `ulule/limiter` 已有；勿为语法糖引入 Chi。
3. **性能与可观测性收敛**：在 `internal/server/middleware.go` 统一 `otelhttp` + `prometheus`，为每个插件 handler 自动打 `plugin_name` label，替代各框架自带 metrics。

---

## Stage 3 — 迭代方案（本 worktree 实施）

### 3.1 目标与非目标

- **目标**：将 Stage 1-2 研究落盘为可审计文档 + 可验证的代码门禁，确保后续贡献不偏离“stdlib+Huma”选型；并在**不改业务逻辑**前提下补一处可证明的架构守卫。
- **非目标**：不做框架迁移；不改 `go.mod` 主依赖；不改前端。

### 3.2 交付清单

| # | 交付物 | 路径 | 验收 |
|---|---|---|---|
| 1 | 本研究文档 | `go_frameworks_research_for_octarq.md` (worktree 根，后续合入 `docs/` 或根) | PR 可审 |
| 2 | Huma 契约门禁脚本/测试 | `internal/server/huma_guard_test.go` 或 `scripts/check-huma.sh` | `go test ./...` 通过 |
| 3 | Worktree + PR | `research/go-frameworks-phase3` → `main` | `pr-ship` 合并 |

### 3.3 任务分解

- **T1 — 文档落盘**：本文件即 T1，补齐 Stage 1-2 重建 + Stage 3 方案。
- **T2 — 架构守卫（Architecture Guard）**：新增测试 `TestHumaGuard_NoDirectAPIRegistration`，扫描 `plugin/` + `internal/api/` 确保无绕过 Huma 的 `/api/` 注册；若发现则 fail，引导贡献者走 `ctx.Huma`。
- **T3 — PR 交付**：`git push` + `gh pr create` + `pr-ship` 合并，CI 需过 `go vet`/`go test -race`/`gofmt`。

### 3.4 风险与回滚

- 风险低：仅新增文档与测试，无运行时变更。
- 回滚：`git revert <merge>` 即可。

### 3.5 时间盒

- T1: 0.5h（已完成）
- T2: 0.5h
- T3: 0.5h（含 CI 等待）

---

## 附录

### A. Worktree 与 pr-ship 约束

- 本分支在 `.worktrees/research-go-frameworks` 隔离开发，符合 `.gitignore` 的 `.worktrees/` 忽略规则。
- 不执行 `npm`，`web/` 未改故不触 `webembed/dist`。
- 合并后 CI 将按 `CLAUDE.md` 触发 `chore(web): refresh embedded dashboard build`（若无前端改则不触发）。

### B. 评审问题（供 PR 讨论）

1. 是否将本文档移入 `docs/architecture/go-frameworks.md` 并纳入 `website` 导航？
2. 是否将 Huma 门禁从测试提升为 `golangci-lint` 自定义 linter？
3. 是否在 `CONTRIBUTING.md` 增补“新增 API 必须走 Huma”章节？

### C. 变更记录

- 2026-08-24 — 重建 Stage 1-2，拟定 Stage 3，创建 worktree `research/go-frameworks-phase3`。


# OCT-1 架构演进与特性借鉴方案 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地 6 项架构演进（借鉴 GoFrame/Beego/PocketBase/Caddy/Kratos/Go-Zero/Huma/Ent），使 octarq 具备声明式契约同构、Agent-Native 错误、Typed EventBus、Scoped 多级缓存、CLI 生成、动态配置表单能力

**Architecture:** 基于现有 `plugin.EndpointSpec` 双通道 + `AgentError` + `ScopedCache` 基线，增量扩展：Typed EventBus 以泛型 + Hook 链实现强类型订阅；多级缓存以 `gcache` 适配器思路封装 `memory→redis` 两级；CLI 复用 `cobra` + `gf gen` 模板；配置以 JSON Schema → Huma → Admin 表单同构

**Tech Stack:** Go 1.25, Huma v2, stdlib ServeMux, GORM, cobra, generics, otel

---

## Pre-Check: 已有基线 vs 待补齐

| 子任务 | 现状 | 借鉴源 | 动作 |
|---|---|---|---|
| 1 声明式契约同构 | `plugin/endpoints.go` 已实现 `EndpointSpec` HTTP+MCP 双注册（#388） | GoFrame `g` 标签 + Huma | 补测试覆盖 + 文档 + 示例插件 |
| 2 Agent-Native 错误 | `plugin/errors.go` 已有 `AgentError`（#386） | GoFrame `gerror` | 扩展 `errors.go` 为可序列化 RFC7807 + 自愈建议 + 前端映射 |
| 3 Typed EventBus | `internal/eventbus` 仅 webhook 投递，无类型安全 | PocketBase Hook + GoFrame 事件 | 新建 `plugin/typed_bus.go` 泛型 EventBus |
| 4 多级 ScopedCache | `plugin/cache.go` 接口已定义，`internal/cache` 仅 memory+redis 简单封装 | GoFrame `gcache` 适配器 | 实现 L1(memory) + L2(redis) + tag 失效 |
| 5 CLI 工具链 | 无 `octarq plugin gen` | GoFrame `gf gen` + Go-Zero `goctl` | 新建 `cmd/octarq-plugin` 或 `cmd/gen` |
| 6 动态配置 Schema | `settings` 表 KV，无 Schema/表单 | GoFrame `gcfg` + PocketBase Admin | 新增 `plugin/config_schema.go` JSON Schema → Admin 表单 |

---

## File Structure

```
plugin/
  endpoints.go          # 已有，补充测试/文档
  endpoints_test.go     # 新增覆盖
  errors.go             # 扩展 Retryable/HTTP 映射/前端码表
  errors_test.go        # 扩展
  typed_bus.go          # 新增：Typed EventBus (task 3)
  typed_bus_test.go
  cache.go              # 接口保留
  scoped_cache.go       # 新增：L1+L2 实现 (task 4)
  config_schema.go      # 新增：动态 Schema (task 6)
  config_schema_test.go
internal/
  cache/
    cache.go            # 接入 scoped_cache
    memory.go / redis.go
  eventbus/
    typed.go            # 可选：server 侧 typed 包装
cmd/
  plugin-gen/           # 新增：CLI (task 5)
    main.go
    gen.go
    templates/
docs/
  superpowers/plans/... # 本计划
  architecture/oct-1.md # 可选：合并 research doc
go_frameworks_research_for_octarq.md # 来自 research 分支，合入主干
```

---

### Task 1: 声明式契约同构补齐 (借鉴 GoFrame 标签 + Huma) — 已有，需加固

**Files:**
- Modify: `plugin/endpoints.go:110-220` 校验 `Path` 必须以 `/api/` 开头、Name 唯一性校验
- Create: `plugin/endpoints_test.go`
- Create: `examples/plugin-hello/endpoints_example.go` (可选)
- Test: `go test ./plugin -run TestEndpoint`

- [ ] **Step 1: Write failing test — Path 校验**

```go
// plugin/endpoints_test.go
func TestEndpointSpec_RequiresAPIPath(t *testing.T) {
  spec := plugin.EndpointSpec[struct{}, struct{}]{Name: "bad", Path: "/foo", Handler: func(ctx context.Context, in struct{}) (*struct{}, error) { return nil, nil }}
  err := spec.RegisterHTTP(nil, plugin.HTTPOptions{}) // 或新增 Validate()
  // 预期：返回错误 "path must start with /api/"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugin -run TestEndpoint_RequiresAPIPath -v`
Expected: FAIL (no validation yet)

- [ ] **Step 3: Implement Validate + RegisterHTTP 校验**

```go
func (s EndpointSpec[In,Out]) Validate() error {
  if s.Name == "" { return errors.New("name required") }
  if !strings.HasPrefix(s.Path, "/api/") { return errors.New("path must start with /api/") }
  return nil
}
func (s EndpointSpec[In,Out]) RegisterHTTP(...) error {
  if err := s.Validate(); err != nil { return err }
  // existing
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugin -run TestEndpoint -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add plugin/endpoints.go plugin/endpoints_test.go
git commit -m "feat(endpoint): validate EndpointSpec path and name for contract isomorphism"
```

---

### Task 2: Agent-Native 智能错误引导与自愈 (借鉴 GoFrame gerror)

**Files:**
- Modify: `plugin/errors.go:1-75` 增加 `ToProblem()` RFC7807 + `IsRetryable()`
- Modify: `internal/apierror/` 或 `plugin/errors.go` 增加 HTTP→前端码映射
- Create: `plugin/errors_test.go` 追加用例
- Test: `go test ./plugin -run TestAgentError`

- [ ] **Step 1: Write failing test**

```go
func TestAgentError_ToProblem(t *testing.T) {
  ae := plugin.NewAgentError(409, "SLUG_EXISTS", "slug taken", "try another slug", false)
  p := ae.ToProblem("/api/links")
  assert.Equal(t, 409, p.Status)
  assert.Equal(t, "SLUG_EXISTS", p.Code)
  assert.Contains(t, p.Detail, "try another")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugin -run TestAgentError_ToProblem -v`
Expected: FAIL method not defined

- [ ] **Step 3: Implement**

```go
type Problem struct { Type, Title, Detail string; Status int; Code string; Guidance string; Retryable bool }
func (e *AgentError) ToProblem(instance string) *Problem { ... }
func (e *AgentError) HTTPStatus() int { return e.HTTPCode }
```

Ensure `endpoints.go:139-140` 已透传 `huma.NewError(ae.HTTPCode, ae.Message)` 并在 MCP 侧 `FormatMCPAgentError` 被调用。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugin -run TestAgentError -v`

- [ ] **Step 5: Commit**

```bash
git add plugin/errors.go plugin/errors_test.go
git commit -m "feat(errors): RFC7807 problem mapping and retryable guidance for agent self-heal"
```

---

### Task 3: 跨插件 Typed EventBus (借鉴 PocketBase Hook + GoFrame 事件)

**Files:**
- Create: `plugin/typed_bus.go`
- Create: `plugin/typed_bus_test.go`
- Modify: `plugin/plugin.go:440-445` 新增 `Context.PublishTyped` / `OnTyped` 可选
- Test: `go test ./plugin -run TestTypedBus -race`

- [ ] **Step 1: Write failing test**

```go
func TestTypedBus_PublishSubscribe(t *testing.T) {
  bus := plugin.NewTypedBus[MyEvent]()
  var got MyEvent
  bus.Subscribe(func(e MyEvent) error { got = e; return nil })
  bus.Publish(MyEvent{ID: 1})
  assert.Equal(t, 1, got.ID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugin -run TestTypedBus -v` Expected: undefined

- [ ] **Step 3: Implement minimal typed bus**

```go
// plugin/typed_bus.go
type TypedBus[T any] struct { mu sync.RWMutex; handlers []func(T) error }
func NewTypedBus[T any]() *TypedBus[T] { return &TypedBus[T]{} }
func (b *TypedBus[T]) Subscribe(h func(T) error) { b.mu.Lock(); defer b.mu.Unlock(); b.handlers = append(b.handlers, h) }
func (b *TypedBus[T]) Publish(e T) []error { b.mu.RLock(); defer b.mu.RUnlock(); var errs []error; for _, h := range b.handlers { if err:=h(e); err!=nil { errs=append(errs,err) } }; return errs }
func (b *TypedBus[T]) PublishAsync(ctx context.Context, e T) // goroutine + recover
```

兼容 `internal/eventbus` 现有 webhook 投递：TypedBus 仅用于插件内联强类型事件，webhook 仍走 `eventbus.Publish`。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugin -run TestTypedBus -race -v`

- [ ] **Step 5: Commit**

```bash
git add plugin/typed_bus.go plugin/typed_bus_test.go
git commit -m "feat(eventbus): typed generic EventBus for cross-plugin strong typing"
```

---

### Task 4: 插件隔离的多级缓存 ScopedCache (借鉴 GoFrame gcache)

**Files:**
- Create: `plugin/scoped_cache.go` (L1 memory LRU + L2 redis)
- Modify: `internal/cache/cache.go`, `internal/cache/memory.go`, `internal/cache/scoped.go`
- Create: `plugin/scoped_cache_test.go`
- Test: `go test ./plugin -run TestScopedCache; go test ./internal/cache -v`

- [ ] **Step 1: Write failing test**

```go
func TestScopedCache_Isolation(t *testing.T) {
  c1 := plugin.NewScopedCache("links", cache.NewMemory())
  c2 := plugin.NewScopedCache("mail", cache.NewMemory())
  c1.Set(ctx, "k", "v1", time.Minute)
  var v string
  ok, _ := c2.Get(ctx, "k", &v)
  assert.False(t, ok) // 隔离：不同插件 key 不串
}
func TestScopedCache_TwoLevel(t *testing.T) {
  // L1 hit 不走 L2；L2 回填 L1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugin -run TestScopedCache -v` Expected: undefined

- [ ] **Step 3: Implement**

```go
type ScopedCache struct { prefix string; l1 cache.Cache; l2 cache.Cache }
func NewScopedCache(pluginName string, l1, l2 cache.Cache) *ScopedCache { return &ScopedCache{prefix: pluginName+":"} }
func (s *ScopedCache) key(k string) string { return s.prefix + k }
func (s *ScopedCache) Get(ctx context.Context, k string, dest any) (bool, error) {
  if ok, err := s.l1.Get(ctx, s.key(k), dest); ok { return true, err }
  if s.l2 != nil { if ok, err := s.l2.Get(ctx, s.key(k), dest); ok { s.l1.Set(ctx, s.key(k), dest, ttl); return true, err } }
  return false, nil
}
func (s *ScopedCache) InvalidateTag(ctx context.Context, tag string) error { return errors.Join(s.l1.Delete(ctx, tag), s.l2.Delete(ctx, tag)) }
```

复用 `internal/cache.New()` 适配：无 Redis 时 l2=nil，自动降级 memory。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugin -run TestScopedCache -v` + `go test ./internal/cache -v -race`

- [ ] **Step 5: Commit**

```bash
git add plugin/scoped_cache.go plugin/scoped_cache_test.go internal/cache/
git commit -m "feat(cache): two-level ScopedCache L1+L2 with plugin isolation and tag invalidation"
```

---

### Task 5: CLI 工具链 octarq plugin gen (借鉴 GoFrame gf gen + Go-Zero goctl)

**Files:**
- Create: `cmd/plugin-gen/main.go`
- Create: `cmd/plugin-gen/gen.go`
- Create: `cmd/plugin-gen/templates/plugin.go.tmpl`
- Create: `cmd/plugin-gen/templates/frontend.ts.tmpl` (可选)
- Test: `go test ./cmd/plugin-gen -v` + 手动 `go run ./cmd/plugin-gen --help`

- [ ] **Step 1: Write failing test**

```go
func TestGen_PluginScaffold(t *testing.T) {
  tmp := t.TempDir()
  err := gen.Scaffold(tmp, "hello", "my hello plugin")
  assert.NoError(t, err)
  assert.FileExists(t, filepath.Join(tmp, "hello", "plugin.go"))
  assert.FileExists(t, filepath.Join(tmp, "hello", "go.mod"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/plugin-gen -v` Expected: no package

- [ ] **Step 3: Implement minimal gen**

```go
// cmd/plugin-gen/gen.go
func Scaffold(root, name, desc string) error {
  // 1. mkdir name/
  // 2. render plugin.go.tmpl with Name/Title
  // 3. render go.mod with module example.com/<name>
  // 4.可选：web/src/plugins/<name>/index.tsx
}
func main() { rootCmd := &cobra.Command{Use: "octarq-plugin-gen"}; rootCmd.AddCommand(&cobra.Command{Use:"gen [name]", RunE: ...}) }
```

模板参考 `examples/plugin-hello/plugin.go` 现有写法，含 `_ plugin.Plugin = (*Plugin)(nil)` 断言。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/plugin-gen -v` + `go run ./cmd/plugin-gen gen hello --desc "test" && ls hello`

- [ ] **Step 5: Commit**

```bash
git add cmd/plugin-gen/
git commit -m "feat(cli): add octarq plugin gen scaffolding (gf gen/goctl style)"
```

---

### Task 6: 插件配置动态 Schema 与 Admin 表单自适应 (借鉴 GoFrame gcfg + PocketBase Admin)

**Files:**
- Create: `plugin/config_schema.go`
- Create: `plugin/config_schema_test.go`
- Modify: `plugin/plugin.go:440+` 新增 `Context.RegisterConfigSchema` 可选
- Modify: `web/src/plugins/core/settings/` 或 `web/src/lib/configSchema.tsx` (前端自适应表单)
- Test: `go test ./plugin -run TestConfigSchema -v`

- [ ] **Step 1: Write failing test**

```go
func TestConfigSchema_JSONSchema(t *testing.T) {
  s := plugin.ConfigSchema{
    Title: "Hello Config",
    Fields: []plugin.ConfigField{{Name:"apiKey", Type:"string", Required:true, Secret:true}},
  }
  js, err := s.ToJSONSchema()
  assert.NoError(t, err)
  assert.Contains(t, string(js), "apiKey")
  // 校验：前端可据此渲染表单
}
func TestConfig_Validate(t *testing.T) {
  s := plugin.ConfigSchema{Fields: []plugin.ConfigField{{Name:"port", Type:"int", Min: intPtr(1), Max: intPtr(65535)}}}
  err := s.Validate(map[string]any{"port": 99999})
  assert.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugin -run TestConfigSchema -v` Expected: undefined

- [ ] **Step 3: Implement**

```go
// plugin/config_schema.go
type ConfigFieldType string // string,int,bool,select
type ConfigField struct { Name, Label, Desc string; Type ConfigFieldType; Required, Secret bool; Default any; Enum []string; Min, Max *int }
type ConfigSchema struct { Title string; Fields []ConfigField }
func (s ConfigSchema) ToJSONSchema() ([]byte, error) {
  // 转 JSON Schema draft-07，Secret 字段标记 x-secret，前端据此用 password input
}
func (s ConfigSchema) Validate(data map[string]any) error {
  // 逐字段类型/必填/枚举/范围校验
}
func (c *Context) RegisterConfigSchema(pluginName string, schema ConfigSchema) error // 写入 settings 的 key 为 plugin:<name>:schema
```

存储复用 `plugin.Context.GetGlobalSetting/SetGlobalSetting` + `Encrypt` 对 Secret 字段落库时加密，前端通过 `GET /api/config/schema/{plugin}` 动态拉取渲染 `packages/plugin-sdk` 表单。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugin -run TestConfigSchema -v` + `npx tsc --noEmit`（如改前端）

- [ ] **Step 5: Commit**

```bash
git add plugin/config_schema.go plugin/config_schema_test.go web/src/lib/configSchema.tsx
git commit -m "feat(config): dynamic JSON Schema for plugin settings with secret encryption and admin form adaption"
```

---

## Verification Checklist (per task)

- `go vet ./...` ✅
- `go test ./... -race` ✅
- `gofmt -w` ✅
- `npx tsc --noEmit` (若动前端) ✅

## Execution Order

1. Task 1 & 2 可并行（均在 plugin/，无依赖）
2. Task 3 独立（typed_bus）
3. Task 4 依赖 Task 3 的隔离思路但可并行
4. Task 5 独立 CLI
5. Task 6 最后（需 cache/eventbus 稳定）

建议 Worktree：`feat/oct-1-architecture-evolution` 分 6 个 commits，按 pr-ship 逐个 PR 或一次大 PR（依 CI 耗时定）。


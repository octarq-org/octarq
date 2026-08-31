---
title: Conventions
description: Single source of truth for code naming, boundaries, state management, and i18n glossary.
sidebar:
  order: 1
  group:
    label: "Developers"
---

This document establishes the single source of truth for engineering conventions, architectural boundaries, security invariants, and localization guidelines across Octarq core and plugins.

---

## 1. Code & Routing Naming

### Route Scopes & Namespaces

Octarq distinguishes between three routing scopes:

| Scope | URL Prefix | Purpose & Execution Context |
|---|---|---|
| **Tenant Scope** | `/admin/...` | Workspace-level dashboard and features. Managed under the tenant shell and gated per workspace. |
| **Instance Scope** | `/instance/...` | Deployment-wide operational console. Managed under the instance console and restricted to instance admins. Flat navigation. |
| **Plugin API Namespace** | `/api/x/{plugin-name}/...` | Dedicated isolated API namespace for third-party and out-of-tree plugins. |

#### Third-Party Plugin API Isolation

`http.ServeMux` panics on duplicate route registrations. To prevent route collisions between plugins or with future core routes, out-of-tree plugins **must** mount backend routes under:

```
/api/x/{plugin-name}/...
```

For example, a plugin named `stripe` must register `POST /api/x/stripe/webhooks`, not `POST /api/webhooks`.

In-tree core routes (`/api/domains`, `/api/emails`, `/api/products`, etc.) retain top-level nouns. Routes outside `/api/` (such as public landing pages or OAuth callback handlers like `/auth/callback/{provider}`) are permitted when necessary.

### File & Component Naming

- **React Components**: Use `PascalCase` with `.tsx` extension (e.g., `DomainList.tsx`, `MailboxDetail.tsx`, `SettingsModal.tsx`).
- **Utilities & Helpers**: Use `kebab-case.ts` (e.g., `safe-url.ts`, `format-bytes.ts`, `crypto-helpers.ts`).
- **React Hooks**: Use `useCamelCase.ts` or `useCamelCase.tsx` (e.g., `usePluginGate.ts`, `useWorkspace.ts`).
- **Styling Tokens & Theme**: Use semantic Tailwind utility classes and CSS variables (e.g., `var(--nb-*)`, `bg-glass`, `border-border/40`, `text-muted-foreground`). Never hardcode raw hex values or unstyled HTML elements.

### Go Naming & Interface Assertions

- **Package Names**: Short, lowercase, single-word nouns matching the domain (e.g., `package mail`, `package dns`, `package links`, `package idempotency`).
- **HTTP Handlers**: Descriptive verb-noun functions (e.g., `handleCreateLink`, `listDomainsHandler`, `GetMailbox`).
- **Compile-Time Interface Assertions**: Every plugin must explicitly declare compile-time interface assertions for `plugin.Plugin` and all optional interfaces it implements:

```go
package hello

import (
    "context"
    "net/http"
    "github.com/octarq-org/octarq/plugin"
)

type Plugin struct{}

// Ensure Plugin satisfies implemented interfaces at compile time
var (
    _ plugin.Plugin             = (*Plugin)(nil)
    _ plugin.MenuProvider       = (*Plugin)(nil)
    _ plugin.InstanceMenuProvider = (*Plugin)(nil)
    _ plugin.Starter            = (*Plugin)(nil)
)

func (p *Plugin) Name() string { return "hello" }
func (p *Plugin) Models() []any { return nil }
func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) { /* ... */ }
func (p *Plugin) Menus() []plugin.MenuItem { /* ... */ }
func (p *Plugin) InstanceMenus() []plugin.MenuItem { /* ... */ }
func (p *Plugin) Start(ctx context.Context) { /* ... */ }
```

> [!NOTE]
> Optional interfaces (such as `MenuProvider`, `Starter`, `MCPProvider`) are discovered at boot time via runtime type assertion. Without compile-time assertions, a typo in a method signature fails silently.

---

## 2. Package & Architecture Boundaries

```
┌────────────────────────────────────────────────────────┐
│                   Client / Dashboard                   │
│         (React UI + @octarq/plugin-sdk facade)         │
└───────────────────────────┬────────────────────────────┘
                            │ HTTP / JSON API
                            ▼
┌────────────────────────────────────────────────────────┐
│                   Host Core Engine                     │
│    (Session Auth, RBAC, Safe Router, Auto-Gate Mux)    │
└───────────────────────────┬────────────────────────────┘
                            │ plugin.Context
                            ▼
┌────────────────────────────────────────────────────────┐
│                 Plugin Execution Layer                 │
│      DB (GORM) │ Encrypt │ SafeHTTP │ Service Registry │
└────────────────────────────────────────────────────────┘
```

### Import Whitelist & Blacklist

- **Forbidden Imports**: Plugins must **never** import `internal/*` or `models/*` packages from core or other plugins.
- **Allowed Facade**: All host resources must be accessed exclusively through `*plugin.Context`:
  - `ctx.DB`: Scoped database access with GORM AutoMigrate support.
  - `ctx.Guard`: Standard authentication and tenant verification middleware.
  - `ctx.Encrypt` / `ctx.Decrypt`: AES-256-GCM authenticated encryption for sensitive settings.
  - `ctx.Audit`: Structured audit logging.
  - `ctx.Notify`: Notification dispatching to tenant users and instance admins.
  - `ctx.SendMail` / `ctx.OnEmail`: Inbound and outbound email hooks.
  - `ctx.DNS`: Programmatic DNS zone and record management.
  - `ctx.UserID(r)` / `ctx.OrgID(r)`: Request-scoped user and organization identifiers.
  - `ctx.GetWorkspaceSetting` / `ctx.SetWorkspaceSetting`: Workspace-scoped key-value configuration.

### Model & Table Ownership

- Each plugin owns its database models declared via `Models() []any`.
- During application startup, `AutoMigrate` runs once across all registered models. A startup preflight check rejects duplicate non-core table names across different plugins.
- To read or reference core data without importing `internal/models`, declare a local mirror struct within the plugin package and override `TableName() string`.

### Inter-Plugin Communication via Service Registry

Hard imports between plugins are strictly prohibited. Plugins collaborate via the in-memory Service Registry on `plugin.Context`:

1. **Register a Service (during `Mount`)**:
   ```go
   ctx.Provide("analytics.tracker", myTrackerInstance)
   ```
2. **Resolve a Service Lazily (in `Start` or per-request)**:
   ```go
   if tracker, ok := plugin.LookupAs[Tracker](ctx, "analytics.tracker"); ok {
       tracker.TrackEvent(event)
   }
   ```

Duplicate service names trigger a startup error. Consumers must resolve dependencies lazily and handle missing services gracefully.

### Frontend SDK Isolation

- Frontend plugin modules must only depend on `@octarq/plugin-sdk` (and peer `react`).
- Do not import host-internal files or bypass SDK abstractions.
- All plugin page routes **must** use `React.lazy` dynamic imports (`lazy(() => import("./Page"))`) to ensure zero bundle overhead when a plugin is excluded.

---

## 3. State Management & API Boundary Rules

### Server State vs UI State Separation

- **Server State (Single Source of Truth)**: Server data must be managed and cached exclusively through standard query hooks and API client calls.
- **No Duplicate Global Stores**: Never copy server API responses into global UI stores (e.g., Redux, Zustand, or global Context) to avoid split-brain state and desynchronization drift.
- **UI State**: Confine client-side state to ephemeral UI interactions (modal open/close states, active filter tabs, unsubmitted form drafts).

### Graceful Degradation Strategy

Never bubble uncaught errors or raw HTTP status codes to users. Route components are wrapped in `PluginGate` and should degrade gracefully:

| HTTP Status | Trigger / Scenario | Expected UI Behavior |
|---|---|---|
| **402 Payment Required** | Route or feature requires an upgraded tier / license. | Render `<LockedFeature />` or open the tier upsell modal. |
| **403 Forbidden** | User lacks required tenant role (e.g., Member vs Admin). | Render access-denied state with clear remediation. |
| **404 Not Found** | Plugin is disabled by workspace settings or unmounted. | Render neutral "Feature not available in this build" state. |

Use `usePluginGate().degrade(err.status)` inside custom pages to trigger standardized degradation views.

### Write Idempotency

All endpoints that create resources, process payments, or trigger external mutations must support the `Idempotency-Key` HTTP header.

Use the host idempotency middleware via the service registry:

```go
import "github.com/octarq-org/octarq/idempotency"

type middleware = func(http.Handler) http.Handler

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
    idem := func(h http.Handler) http.Handler { return h }
    if m, ok := plugin.LookupAs[middleware](ctx, idempotency.ServiceName); ok {
        idem = m
    }
    mux.Handle("POST /api/x/hello/records", idem(ctx.Guard(createRecordHandler)))
}
```

- **Replay**: Duplicate requests with the same key and identical body within 24 hours replay the original cached response with `Idempotency-Replayed: true`.
- **Payload Mismatch**: Reusing a key with a different payload returns `422 Unprocessable Entity`.
- **Concurrent Requests**: In-flight concurrent requests with the same key return `409 Conflict` with `Retry-After`.

---

## 4. Security Invariants

### 1. Instant Session Revocation

When a user's role is changed, their permissions are modified, or they are removed from an organization/workspace, all active `user_sessions` for that user within that tenant must be revoked immediately in both the database and memory cache.

### 2. Outbound Request Protection (SSRF & DNS Rebinding)

Any outbound HTTP request where the destination URL is supplied or influenced by users, tenants, webhooks, or external OIDC discovery **must** use `plugin/safehttp`:

```go
import "github.com/octarq-org/octarq/plugin/safehttp"

client := safehttp.NewClient(10 * time.Second)
resp, err := safehttp.Get(ctx, client, targetURL, "Octarq-Bot/1.0")
```

`safehttp` performs IP validation at **socket dial time**, effectively blocking:
- Loopback addresses (`127.0.0.0/8`, `::1`).
- Cloud metadata endpoints (`169.254.169.254`).
- Private RFC1918 networks (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`).
- DNS rebinding attacks and malicious redirects into private infrastructure.

### 3. Secrets & Key Storage

Sensitive configuration items (API tokens, OAuth client secrets, webhook signing keys, SMTP credentials) must be encrypted before persistence using AES-256-GCM via `ctx.Encrypt` / `ctx.Decrypt`. Plaintext secret storage is strictly prohibited.

---

## 5. i18n Glossary & Voice Guide

### Terminology Glossary

To maintain consistent terminology across the dashboard, CLI, and documentation, adhere to the following glossary:

| English Term | 中文术语 | 规范说明 / 适用范围 |
|---|---|---|
| `Workspace` | 工作区 | 租户级逻辑隔离单元与团队空间 |
| `Domain` | 域名 | 自定义域名、托管域名与解析记录 |
| `Mailbox` | 邮箱 | 接收与发送邮件的虚拟邮箱地址 |
| `Short Link` | 短链接 | 链接缩短、跳转重定向与点击追踪 |
| `Audit Log` | 审计日志 | 安全合规、访问记录与操作日志 |
| `API Token` | API 令牌 | 开发者凭证与服务调用密钥 |
| `Instance` | 实例 | 单个 Octarq 部署节点或系统全局 |
| `Plugin` | 插件 | 可扩展的后端与前端功能模块 |
| `Role` | 角色 | 权限身份（`Owner` 所有者, `Admin` 管理员, `Member` 成员） |
| `Idempotency Key` | 幂等键 | 防止重复提交的唯一请求标识 |

> [!NOTE]
> Technical identifiers, protocol names, and database engines remain lowercase in English: `mcp`, `sqlite`, `postgres`, `idempotency_key`, `token`.

### Typography & Mixed Spacing

- **Spacing**: Always insert one half-width space between Chinese characters and English words, numbers, or acronyms.
  - *Correct*: 在 SQLite 数据库中配置 MCP 工具。
  - *Incorrect*: 在SQLite数据库中配置MCP工具。
- **Punctuation**: Use standard full-width Chinese punctuation (`，` `。` `：` `！` `？` `（` `）`) in Chinese content. Use straight double quotes (`"..."`) or markdown code formatting for technical names.
- **Button & Action Copy**: Use concise, verb-first Chinese labels testable at 2–4 characters:
  - `保存修改` (Save Changes)
  - `创建域名` (Create Domain)
  - `立即刷新` (Refresh Now)
  - `删除成员` (Remove Member)
  - `复制链接` (Copy Link)
- **Error Messages**: Write clear, calm, and constructive messages explaining the root cause and providing actionable next steps. Avoid technical jargon or accusatory language.

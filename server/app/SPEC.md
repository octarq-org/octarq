# Module Specification: App Composition Root (`server/app`)

## 1. Architectural Overview & Role
`server/app` is the public composition root of Octarq (Core-as-Library). It orchestrates configuration, database connections, cryptographic envelopes, authentication, background task queues, telemetry, cron schedulers, health collectors, and the unified HTTP / MCP servers.
It defines and provisions the Host SPI (`plugin.Host`) that enables external extensions and Pro modules to mount into Octarq without forking.

- **Package**: `app` (`server/app`)
- **Key Responsibilities**:
  - Composition root: `New()` opens configuration and database without migrating; `Run()` and `RunMCP()` execute unified migrations and launch runtimes.
  - Preflight validations: verifies no name collisions (`preflightNameCollisions`), missing dependencies (`preflightDependencies`), or table conflicts (`preflightTableCollisions`) prior to execution.
  - Unified plugin context creation (`buildPluginContext`): injects `plugin.Host`, `TenantDB`, `Guard`, `Cache`, `DNS`, `Crypto`, `EventSpine`, and logging into every mounted plugin.
  - Fail-closed route gating (`pluginGate`, `gatedMux`, `gatedAPI`): returns 404 Fail-Closed when a tenant workspace does not have an optional plugin enabled.
  - MCP Server lifecycle (`RunMCP`): provisions the same plugin dependencies over standard I/O for AI agent tooling.
- **Entitlement Tier**: Core OSS Infrastructure.
- **Binary Embedding Guarantee**: Co-located at `server/app/SPEC.md`. Strictly excluded from binary embedding.

---

## 2. Exact Go Types, Structs & Interface Assertions

### 2.1 Host SPI Implementations (`server/app/host_runtime.go` & `server/app/host_services.go`)
```go
var _ plugin.Host = (*hostRuntime)(nil)
var _ plugin.HostSession = (*hostSession)(nil)
var _ plugin.CryptoVault = (*hostCryptoVault)(nil)
var _ plugin.SettingsStore = (*hostSettingsStore)(nil)
var _ plugin.EventSpine = (*hostEventSpine)(nil)
```

### 2.2 Core Structures
- **App**:
  ```go
  type App struct {
      cfg             *config.Config
      gdb             *gorm.DB
      cipher          *crypto.Cipher
      auth            *auth.Manager
      geo             *geo.Resolver
      plugins         []plugin.Plugin
      services        *plugin.Registry
      healthCollector atomic.Pointer[monitor.Collector]
      webFS           fs.FS
      telemetry       *telemetry.Telemetry
  }
  ```

---

## 3. Lifecycle & State Machine

1. **Instantiation (`New`)**:
   - Loads environment and file configuration.
   - Opens SQL database connection (`db.Open`).
   - Configures AES-GCM cipher and decryptor hooks.
   - Initializes GeoIP resolver and OpenTelemetry tracing.
2. **Registration (`Use`)**:
   - Registers external plugins into `a.plugins`.
3. **Execution (`Run` / `RunMCP`)**:
   - Preflight collision and dependency validation.
   - Schema AutoMigrate pass with both core models and all plugin models.
   - Cipher envelope mode activation (`EnableEnvelope`).
   - Unified plugin context construction (`buildPluginContext`).
   - Route and huma API registration through `routeRegistry` conflict checking.
   - Reactor startup (`eventbus.StartReactors`).
   - HTTP server bind and graceful shutdown handling.

---

## 4. Fail-Closed Invariants & Non-Functional Requirements

- **Hard Line Ceiling**: Every production and test file in `app/` strictly < 500 LOC.
- **Fail-Closed Gate**: Plugin routes return HTTP 404 immediately when disabled for the target workspace.
- **Table Collision Fail-Closed**: Any two plugins claiming the same GORM table fail startup immediately.
- **Route Collision Fail-Closed**: Any overlapping route registrations fail startup immediately.
- **Concurrency Safety**: Verified with `-race` across entire test suite.

---
title: Writing a Plugin
description: Step-by-step guide to writing a custom Octarq plugin in 30 minutes.
---

Octarq is extended by **plugins**, not forks. A plugin is a self-contained feature composed at build time, consisting of:

- A **Go module** implementing the backend contract `plugin.Plugin`
- A **JS package** implementing the frontend contract `UIPlugin` (from `@octarq/plugin-sdk`)

---

## 1. Directory Structure

A minimal full-stack plugin repository follows this structure:

```
your-plugin/
├── go.mod                 # Go module (e.g., github.com/you/octarq-plugin-hello)
├── hello.go               # Implements plugin.Plugin (+ optional MenuProvider, MCPProvider)
└── web/
    ├── index.ts           # Implements UIPlugin (@octarq/plugin-sdk)
    └── Page.tsx           # React UI page
```

---

## 2. Backend Implementation (`plugin.Plugin`)

Every plugin implements `plugin.Plugin` and optionally registers routes, models, menus, or MCP tools.

```go
package hello

import (
    "net/http"
    "github.com/octarq-org/octarq/plugin"
)

type Plugin struct{}

// Name returns a unique, stable ID for the plugin.
func (Plugin) Name() string { return "hello" }

// Models returns GORM model structs owned by this plugin.
func (Plugin) Models() []any { return nil }

// Mount registers HTTP endpoints on the host router.
func (Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
    mux.Handle("GET /api/hello/ping", ctx.Guard(http.HandlerFunc(
        func(w http.ResponseWriter, r *http.Request) {
            w.Write([]byte(`{"message": "pong"}`))
        },
    )))
}

// Menus defines sidebar entries provided by this plugin.
func (Plugin) Menus() []plugin.MenuItem {
    return []plugin.MenuItem{
        {ID: "hello", Label: "Hello", Path: "/hello", Icon: "👋", Category: "Workspace"},
    }
}

// Compile-time interface assertions
var (
    _ plugin.Plugin       = Plugin{}
    _ plugin.MenuProvider = Plugin{}
)
```

---

## 3. Frontend Implementation (`UIPlugin`)

The frontend uses `@octarq/plugin-sdk` to register routes, sidebar items, widgets, and translation dictionaries.

```ts
import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

export const helloPlugin: UIPlugin = {
  name: "hello",
  routes: [
    {
      path: "/hello",
      Component: lazy(() => import("./Page")),
    },
  ],
  menu: [
    {
      id: "hello",
      label: "Hello World",
      path: "/hello",
      icon: "👋",
      category: "Workspace",
    },
  ],
};
```

---

## 4. Inter-Plugin Service Registry

Plugins communicate through an in-memory service registry provided on `plugin.Context`.

- **Provide a service**:
  ```go
  ctx.Provide("hello.service", myServiceInstance)
  ```
- **Lookup a service safely**:
  ```go
  if svc, ok := plugin.LookupAs[MyService](ctx, "hello.service"); ok {
      svc.DoSomething()
  }
  ```

> `Start(ctx context.Context)` from optional `Starter` interface runs in a background goroutine after all plugins have mounted, making it the safe entry point for inter-plugin initialization.

---

## 5. Composition & Building

Octarq plugins are composed at build time (similar to `xcaddy`):

```bash
# Build custom binary with your Go and JS plugin modules
OCTARQ_PLUGINS='[{"go":"github.com/you/octarq-plugin-hello","npm":"@you/octarq-plugin-hello"}]' make plugin-build
```

- **Routes Auto-Gate**: Endpoints automatically return `404` when disabled in workspace settings.
- **AutoMigrate Preflight**: Database tables are safely resolved and migrated at startup.

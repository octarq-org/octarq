---
title: Composable Plugins & Context
description: Details of the plugin dependency contract, build-time exclusion, and frontend self-containment.
---

This document outlines the design for composability, dependency contracts, and building custom editions of Octarq.

## 1. Principles of Composable Plugins

To allow building custom editions, three structural principles are enforced:

1. **Dependency Contract** — Plugins declare what they require; the app refuses to start on an unsatisfiable composition.
2. **Build-Time Exclusion** — An edition can compile out a core feature (e.g., a links-only binary without mail) so that the excluded feature's code is absent from the binary, rather than just unmounted.
3. **Frontend Self-Containment** — Each feature's UI (pages, components, API calls, i18n) lives inside its plugin directory and is composed through the same manifest pipeline Pro plugins use.

## 2. Dependency Declaration & Validation

To enforce dependency safety at startup, the `plugin.Info` struct has a `Requires` field:

```go
type Info struct {
    Title    string
    Version  string
    Requires []string // Slice of plugin names this plugin depends on
}
```

For example:
- `links` plugin declares: `Requires: []string{"dns"}`
- `mail` plugin declares: `Requires: []string{"dns", "links"}`

During application startup, a preflight validation checks that every mounted plugin's dependencies are satisfied by the set of registered plugins. If a dependency is missing, the application halts with a clear error.

## 3. Frontend Feature Self-Containment

To make features truly modular, all frontend code for a feature is located inside its plugin directory (e.g., `web/src/plugins/<feature>/`):
- **`index.ts`** — Contains the `UIPlugin` definition (including routes, widgets, areas, i18n).
- **`pages/` & `components/`** — All page and component files.
- **`api.ts`** — The plugin-specific API client methods.
- **`i18n`** — Localization keys merged dynamically under the plugin's namespace.

This allows completely dropping a feature from the UI bundle by removing its entry in the default manifest (`web/octarq.plugins.json`).

## 4. Custom Edition Build Recipes

Core feature plugins (`dns`, `mail`, `links`) can be selectively excluded from both the backend Go binary and the frontend web UI to produce lightweight, custom-tailored editions of Octarq.

### Backend Composition

Trimmed editions are defined at the compilation root (typically in `main.go`). Instead of auto-mounting all plugins, the composition root explicitly selects the plugins to use:

```go
// Example of a links-only composition root
package main

import (
    "github.com/octarq-org/octarq/app"
    "github.com/octarq-org/octarq/plugins/dns"
    "github.com/octarq-org/octarq/plugins/links"
)

func main() {
    a, _ := app.New()
    a.Use(dns.New())   // Required by links
    a.Use(links.New())
    a.Run()
}
```

Because of Go's dead-code elimination, any unreferenced packages (like `plugins/mail`) are automatically excluded from the final compiled binary by the linker.

### Frontend Manifest Composition

Control which UI plugins are bundled by configuring `web/octarq.plugins.json` or setting `OCTARQ_PLUGINS_MANIFEST` environment variables during build:

Example (`octarq.nomail.json`):
```json
{
  "plugins": [
    "./src/plugins/dns",
    "./src/plugins/links"
  ]
}
```

Build command:
```bash
OCTARQ_PLUGINS_MANIFEST=octarq.nomail.json npm run build
```

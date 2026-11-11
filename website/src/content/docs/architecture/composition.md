---
title: Composition Model
description: How backend and frontend plugins are composed into different Octarq editions without build tags.
sidebar:
  order: 3
  group:
    label: "Architecture"
---


Octarq uses an opt-in composition model across its backend and frontend architecture. Both follow the same design pattern.

## 1. Backend Composition

Rather than using complex Go build tags to conditionally compile features in or out, Octarq relies on explicit composition roots and Go linker dead-code elimination (DCE).

If no reachable code in your entry point (`main.go`) imports a plugin package, that plugin is omitted from the final binary by the compiler and linker.

### Default OSS Composition

Core is infrastructure only. It ships three plugins: help, tenantquery, and notify. It ships no business feature.

In the default open-source entry point (`main.go`), plugins are explicitly imported and mounted:

```go
package main

import (
    "github.com/octarq-org/octarq/server/app"
    "github.com/octarq-org/octarq/server/plugins/builtin"
)

func main() {
    a, _ := app.New()

    // Mount the Core infrastructure plugins
    for _, p := range builtin.Default() {
        a.Use(p)
    }

    a.Run()
}
```

The `builtin.Default()` helper returns the Core infrastructure plugins in dependency order:
```go
package builtin

import (
    "github.com/octarq-org/octarq/server/plugin"
    "github.com/octarq-org/octarq/server/plugins/help"
    "github.com/octarq-org/octarq/server/plugins/notify"
    "github.com/octarq-org/octarq/server/plugins/tenantquery"
)

func Default() []plugin.Plugin {
    return []plugin.Plugin{help.New(), tenantquery.New(), notify.New()}
}
```

### Business Plugin Composition

Business plugins live in the `octarq-plugins` repository. Mount the Core plugins first. Then mount the business plugins:

```go
package main

import (
    "github.com/octarq-org/octarq/server/app"
    "github.com/octarq-org/octarq/server/plugins/builtin"
    "github.com/octarq-org/octarq-plugins/dns"
    "github.com/octarq-org/octarq-plugins/links"
    "github.com/octarq-org/octarq-plugins/mail"
)

func main() {
    a, _ := app.New()

    // 1. Mount the Core infrastructure plugins
    for _, p := range builtin.Default() {
        a.Use(p)
    }

    // 2. Mount the business plugins you want
    a.Use(dns.New())
    a.Use(links.New())
    a.Use(mail.New())

    a.Run()
}
```

Custom out-of-tree plugins use the same mechanism. Add your own `a.Use(yourplugin.New())` call.

## 2. Frontend Composition

The frontend matches this opt-in design using a plugin manifest (`web/octarq.plugins.json`).

A Vite plugin (`web/plugins-manifest.ts`) reads the manifest at build time and generates a virtual module `#octarq-plugins` that registers all active UI plugins:

```ts
// Virtual module generated at build time
import { registerUIPlugin } from "@octarq/plugin-sdk";
import dns from "@octarq/plugin-dns/ui";
import links from "@octarq/plugin-links/ui";

registerUIPlugin(dns);
registerUIPlugin(links);
```

By changing the manifest file, you control exactly which plugin packages are included in the bundle. Unused files are eliminated during Vite's bundling process.

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

In the default open-source entry point (`main.go`), plugins are explicitly imported and mounted:

```go
package main

import (
    "github.com/octarq-org/octarq/server/app"
    "github.com/octarq-org/octarq/server/plugins/builtin"
)

func main() {
    a, _ := app.New()
    
    // Mount the default Core plugins (dns, links, mail)
    for _, p := range builtin.Default() {
        a.Use(p)
    }
    
    a.Run()
}
```

The `builtin.Default()` helper returns the standard set of core plugins in their correct dependency order:
```go
package builtin

import (
    "github.com/octarq-org/octarq/server/plugin"
    "github.com/octarq-org/octarq/server/plugins/dns"
    "github.com/octarq-org/octarq/server/plugins/links"
    "github.com/octarq-org/octarq/server/plugins/mail"
)

func Default() []plugin.Plugin {
    return []plugin.Plugin{dns.New(), links.New(), mail.New()}
}
```

### Custom Plugin Composition

Custom out-of-tree plugins utilize the exact same mechanism. You mount the core plugins, followed by your custom plugins:

```go
package main

import (
    "github.com/octarq-org/octarq/server/app"
    "github.com/octarq-org/octarq/server/plugins/builtin"
    "github.com/your-org/octarq-plugin-analytics"
)

func main() {
    a, _ := app.New()

    // 1. Mount default Core plugins
    for _, p := range builtin.Default() {
        a.Use(p)
    }

    // 2. Mount custom plugins
    a.Use(analytics.New())

    a.Run()
}
```

## 2. Frontend Composition

The frontend matches this opt-in design using a plugin manifest (`web/octarq.plugins.json`). 

A Vite plugin (`web/plugins-manifest.ts`) reads the manifest at build time and generates a virtual module `#octarq-plugins` that registers all active UI plugins:

```ts
// Virtual module generated at build time
import { registerUIPlugin } from "@octarq/plugin-sdk";
import dns from "./src/plugins/dns";
import links from "./src/plugins/links";
import mail from "./src/plugins/mail";

registerUIPlugin(dns);
registerUIPlugin(links);
registerUIPlugin(mail);
```

By changing the manifest file, you control exactly which plugin packages are included in the bundle. Unused files are eliminated during Vite's bundling process.

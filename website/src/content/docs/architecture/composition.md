---
title: Unified Composition Model
description: How backend and frontend plugins are composed into different Octarq editions without build tags.
---

Octarq uses a unified, opt-in composition model across both open-source (OSS) and commercial (Pro) editions. Both backend and frontend follow the same design pattern.

## 1. Unified Backend Composition

Rather than using complex Go build tags to conditionally compile features in or out, Octarq relies on explicit composition roots and Go linker dead-code elimination (DCE).

If no reachable code in your entry point (`main.go`) imports a plugin package, that plugin is omitted from the final binary by the compiler and linker.

### Default OSS Composition

In the default open-source entry point (`main.go`), plugins are explicitly imported and mounted:

```go
package main

import (
    "github.com/octarq-org/octarq/app"
    "github.com/octarq-org/octarq/plugins/builtin"
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
    "github.com/octarq-org/octarq/plugin"
    "github.com/octarq-org/octarq/plugins/dns"
    "github.com/octarq-org/octarq/plugins/links"
    "github.com/octarq-org/octarq/plugins/mail"
)

func Default() []plugin.Plugin {
    return []plugin.Plugin{dns.New(), links.New(), mail.New()}
}
```

### Pro/Commercial Composition

The commercial edition (`octarq-pro`) utilizes the exact same mechanism. It mounts the core plugins, followed by Pro-specific plugins:

```go
package main

import (
    "github.com/octarq-org/octarq/app"
    "github.com/octarq-org/octarq/plugins/builtin"
    "github.com/octarq-org/octarq-pro/plugins/billing"
    "github.com/octarq-org/octarq-pro/plugins/issuer"
)

func main() {
    a, _ := app.New()

    // 1. Mount default Core plugins
    for _, p := range builtin.Default() {
        a.Use(p)
    }

    // 2. Mount Pro plugins
    a.Use(billing.New())
    a.Use(issuer.New())

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

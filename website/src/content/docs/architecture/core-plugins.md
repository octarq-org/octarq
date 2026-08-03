---
title: Core Plugins & Decoupling
description: How core features are extracted into plugins and decoupled using platform seams.
sidebar:
  order: 4
  group:
    label: "Architecture"
---


Octarq isolates features into self-contained plugins. This page documents core plugin extraction and the decoupling seams used.

## Feature Extraction

Core features like **dns**, **links**, and **mail** live in backend plugins (`plugins/`) and frontend plugins (`web/src/plugins/`).

### Ownership boundaries

| Plugin | Model Ownership | Responsibilities | Services Provided |
|--------|-----------------|------------------|-------------------|
| **dns**  | `Domain`, `ProviderAccount`, DNS Records | Domain verification, DNS zone management, API endpoints under `/api/domains` | `dns.manager` (`plugin.DNSManager`), `domain.hosts` (lookup) |
| **links**| `Link`, `LinkEvent` | Dashboard link CRUD under `/api/links`, redirect engine, analytics | `links.wrap` (HTML link rewrite), `links.hostcheck` |
| **mail** | `Mailbox`, `Email`, `Attachment`, `SMTPSender` | Mailbox/SMTP management, inbound email webhook, email dispatch | — (triggers `OnEmail` hooks) |

---

## Decoupling Seams

Architectural seams used by plugins:

### Service Provider Seam (`Context.Provide` / `Lookup`)
When a plugin needs to consume functionality from another plugin (or the core needs it), the target plugin registers its service in `plugin.Context` during `Mount`.
Other components look it up dynamically:
```go
// Register service (in dns plugin Mount)
ctx.Provide("dns.manager", dnsManagerInstance)

// Consume service (in links plugin or core)
if dns, ok := plugin.LookupAs[plugin.DNSManager](ctx, "dns.manager"); ok {
    dns.Verify(domain)
}
```

### Menu Contribution (`MenuProvider`)
Core does not hardcode menus. Plugins implement the `MenuProvider` interface:
```go
type MenuProvider interface {
    Menus() []MenuItem
}
```
The application calls `Menus()` on all active plugins to build the sidebar navigation dynamically.

### Dynamic i18n Namespaces
Plugin translation catalogs (`UIPlugin.i18n`) are registered dynamically under the plugin's namespace. Special top-level keys like `nav` and `settings.pluginDesc` are deep-merged back into the shared global namespace at startup to support translating navigation labels and settings pages without hardcoding terms in the core.

### Static Asset Hosting (`Context.HandleStatic`)
For plugins that need to serve independent single-page apps (SPAs) or static pages (such as the customer portal in Pro), the core provides a generic prefix-based static router seam:
```go
ctx.HandleStatic("/portal", portalDistFS)
```
In the OSS build, requests to `/portal` return a 404, while in Pro builds the portal is served dynamically by the active plugin.

---

## Porting or Creating Core Plugins

When moving endpoints or building a new plugin:

1. **Avoid `internal/*` imports:** Plugins should only import the public `plugin` package. All shared resources (DB, encryption, auditing, config settings) are accessed via `plugin.Context`.
2. **Handle authorization via Context:** Do not hand-authenticate requests within the handler. The core auth middleware authenticates `/api/` calls. Instead, check the active organization:
   ```go
   r, _ := humago.Unwrap(input.Ctx)
   if p.orgID(r) == 0 {
       return nil, huma.Error401Unauthorized("unauthorized")
   }
   ```
3. **Specify compile-time assertions:** Ensure your plugin struct asserts it implements all declared interfaces:
   ```go
   var (
       _ plugin.Plugin       = (*Plugin)(nil)
       _ plugin.MenuProvider = (*Plugin)(nil)
   )
   ```


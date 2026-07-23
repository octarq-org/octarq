---
title: Core Plugins & Decoupling
description: Deep dive into how core features were extracted into plugins and how coupling was resolved.
---

To maintain a clean architectural boundary between the open-source core and commercial extensions, Octarq isolates features into self-contained plugins. This page documents the core plugin extraction and the decoupling seams used.

## 1. Feature Extraction

Core features like **dns**, **links**, and **mail** were historically part of a monolithic API handler. They have been extracted into three separate backend plugins located in `plugins/` and frontend plugins located in `web/src/plugins/`.

### Ownership boundaries:

| Plugin | Model Ownership | Responsibilities | Services Provided |
|--------|-----------------|------------------|-------------------|
| **dns**  | `Domain`, `ProviderAccount`, DNS Records | Domain verification, DNS zone management, API endpoints under `/api/domains` | `dns.manager` (`plugin.DNSManager`), `domain.hosts` (lookup) |
| **links**| `Link`, `LinkEvent` | Dashboard link CRUD under `/api/links`, redirect engine, analytics | `links.wrap` (HTML link rewrite), `links.hostcheck` |
| **mail** | `Mailbox`, `Email`, `Attachment`, `SMTPSender` | Mailbox/SMTP management, inbound email webhook, email dispatch | — (triggers `OnEmail` hooks) |

---

## 2. Decoupling Seams

To prevent the core platform from importing or hardcoding feature-specific logic, several architectural seams are utilized:

### 2.1 Service Provider Seam (`Context.Provide` / `Lookup`)
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

### 2.2 Menu Contribution (`MenuProvider`)
Core does not hardcode menus. Plugins implement the `MenuProvider` interface:
```go
type MenuProvider interface {
    Menus() []MenuItem
}
```
The application calls `Menus()` on all active plugins to build the sidebar navigation dynamically.

### 2.3 Dynamic i18n Namespaces
Plugin translation catalogs (`UIPlugin.i18n`) are registered dynamically under the plugin's namespace. Special top-level keys like `nav` and `settings.pluginDesc` are deep-merged back into the shared global namespace at startup to support translating navigation labels and settings pages without hardcoding terms in the core.

### 2.4 Static Asset Hosting (`Context.HandleStatic`)
For plugins that need to serve independent single-page apps (SPAs) or static pages (such as the customer portal in Pro), the core provides a generic prefix-based static router seam:
```go
ctx.HandleStatic("/portal", portalDistFS)
```
In the OSS build, requests to `/portal` return a clean 404, while in Pro builds the portal is served dynamically by the active plugin.

---

## 3. Developer Guide: Porting or Creating Core Plugins

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

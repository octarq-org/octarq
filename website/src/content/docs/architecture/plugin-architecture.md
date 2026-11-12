---
title: Plugin architecture
description: What a plugin contributes, and the seams it uses to attach to core.
sidebar:
  order: 4
  group:
    label: "Architecture"
---

Octarq has no business feature in core. Every business feature is a plugin that
mounts on one of the seams below. The same seams are open to you.

## What a plugin contributes

| Contribution | Interface | Where it lands |
| --- | --- | --- |
| Database models | `Plugin.Models()` | Joined into the single boot-time migration pass |
| Routes and handlers | `Plugin.Mount(mux, ctx)` | `/api/*` behind the auth middleware |
| Pages | `UIPlugin.routes` | Dashboard shell, at build time |
| MCP tools | `EndpointSpec` with `ExposeMCP` | Tool registry, namespaced per plugin |
| Menu entries | `MenuProvider.Menus()` | Sidebar. This is the only menu source |
| Help pages | `HelpDocsFS` | `/admin/help/` |
| Service for other plugins | `ctx.Provide` | Service registry, looked up by name |

`plugin.DetectedCapabilities` reports which of these a plugin implements, so the
platform never assumes a capability a plugin did not declare.

## Official plugins

Links, mail, DNS, a 2FA vault and the Mail Links demo live in
[octarq-plugins](https://github.com/octarq-org/octarq-plugins). Read them as
reference implementations — they use the same public `plugin` package you do.

| Plugin | Models | Services provided |
| --- | --- | --- |
| `links` | `Link`, `LinkEvent`, `RoutingRule` | `links.resolve`, `links.create` |
| `mail` | `Mailbox`, `Email`, `SMTPSender`, `MailContact`, `MailDraft` | `mail.dispatcher`, `mail.send`, `mail.send.system`, `mail.ready`, `mail.email.get` |
| `dns` | `Domain`, `ProviderAccount`, `DDNSToken` | `dns.manager` |

## Decoupling seams

### Service provider (`Context.Provide` / `Lookup`)

When a plugin needs another plugin's functionality, the owner registers it during
`Mount` and consumers look it up by name:

```go
// Register (dns plugin, during Mount)
ctx.Provide(plugin.ServiceDNSManager, dnsManager)

// Consume (links plugin, or core)
if dns, ok := plugin.LookupAs[plugin.DNSManager](ctx, plugin.ServiceDNSManager); ok {
    dns.Set(ctx, orgID, domainID, record)
}
```

### Menu contribution (`MenuProvider`)

Core hardcodes no menu. A plugin that wants a sidebar entry implements:

```go
type MenuProvider interface {
    Menus() []MenuItem
}
```

The application calls `Menus()` on every active plugin and builds the sidebar from
the result. A plugin cannot add a menu from the frontend.

### Dynamic i18n namespaces

Plugin translation catalogs (`UIPlugin.i18n`) register under the plugin's own
namespace. Two keys, `nav` and `settings.pluginDesc`, deep-merge into the shared
namespace so navigation labels and settings descriptions translate without
hardcoding anything in core.

### Static asset hosting (`Context.HandleStatic`)

A plugin that serves its own SPA mounts it under a path prefix:

```go
ctx.HandleStatic("/portal", portalDistFS)
```

Requests under the prefix serve from the filesystem you pass, with client-side
routing fallback enabled.

## Writing a plugin

1. **Import only the public `plugin` package.** Never import `internal/*`. All
   shared resources (database, encryption, audit, config) come through
   `plugin.Context`.
2. **Let the middleware authenticate.** The core auth middleware already guards
   `/api/`. Check the workspace instead of re-authenticating:
   ```go
   r, _ := humago.Unwrap(input.Ctx)
   if p.orgID(r) == 0 {
       return nil, huma.Error401Unauthorized("unauthorized")
   }
   ```
3. **Assert the interfaces at compile time:**
   ```go
   var (
       _ plugin.Plugin       = (*Plugin)(nil)
       _ plugin.MenuProvider = (*Plugin)(nil)
   )
   ```

See [Writing a plugin](/writing-a-plugin/) for the full walkthrough and
[Composition](/architecture/composition/) for how a build selects plugins.

# Core ↔ Feature-Plugin Decoupling Audit

> Status: requirements/design — none of §2 is implemented yet.
> Date: 2026-07-19
> Scope: everything still hard-coded in the open-source core that belongs to a
> feature plugin (links / mail / dns) or to a commercial (Pro) plugin.

The backend extraction (docs/CORE-PLUGIN-EXTRACTION.md) and the frontend
UIPlugin split are done: links/mail/dns Go code lives in `plugins/`, their UI
lives in `web/src/plugins/` behind the manifest. This audit lists what is
*still* bound to the core, ordered by impact, with the seam each item should
use. Items marked **[done]** were fixed in the same branch as this document.

## 1. Fixed in this branch [done]

- **`ProGate` / `useProGate` compat aliases** (`web/src/plugins/PluginGate.tsx`)
  — removed; nothing imported them. `PluginGate` / `usePluginGate` are the only
  names now.
- **Commercial plugin descriptions hard-coded in core**
  (`web/src/pages/settings/plugins.tsx` `PLUGIN_DESC`, plus the
  `settings.pluginDesc.*` i18n entries for ai/infra/finance/commerce/billing/
  customer/issuer/licensing/portal/product) — removed. The plugin manager now
  localizes only the in-tree Core feature plugins (dns/links/mail) and renders
  every other plugin's backend `Describe()` description as-is.
- **Dead Commerce quick-start copy** (`overview.stepStorefront*` /
  `overview.stepBilling*` in `web/src/i18n/pages/overview.ts`) — removed; no
  consumer in the OSS tree.

## 2. Remaining work (design)

### 2.1 Backend `listMenus` hard-codes feature menus — HIGH

`internal/api/tenant_menu.go` `listMenus` returns `links`, `mail`, `domains`
(and the Infrastructure placeholders `certs`, `databases`, `storage`) as core
menu items, unconditionally. Consequences:

- `/api/menus` announces `/links` even when the links plugin is disabled for
  the workspace — the frontend's orphan filter (menu shown only when a backend
  half announces its path) can't do its job for feature plugins.
- The plugin-menus loop right below it **drops `Order`** when copying
  `plugin.MenuItem` → API `MenuItem` (the API struct has no `order` field), so
  a Go-side plugin can't influence ordering; only the frontend UIPlugin's
  `order` works today.

**Design**: make the links/mail/dns Go plugins implement `MenuProvider`
(`Menus()` already exists on the interface) and delete their rows from the
core list. Core keeps only overview/audit/abuse and the asset placeholders it
truly owns. Add `Order int \`json:"order,omitempty"\`` to the API `MenuItem`
and copy it in the loop. The `pluginActive` filter then hides a disabled
plugin's menu at the source, and the frontend orphan filter becomes correct
for feature plugins.

### 2.2 Feature settings pages live in the core shell — HIGH

Three core files are consumed *only* by feature UI plugins (imported via
`../../../pages/Settings` re-exports):

| Core file | Exports | Sole consumer |
|---|---|---|
| `web/src/pages/settings/linkMail.tsx` | `LinkSettings`, `MailSettings` | links + mail plugin pages |
| `web/src/pages/settings/smtp.tsx` | `SMTPSenders` | mail plugin pages |
| `web/src/pages/settings/providers.tsx` | `ProviderAccounts` | dns plugin pages |

**Design**: move each file into its owning plugin
(`web/src/plugins/links|mail|dns/pages/…`), split `LinkSettings` from
`MailSettings`, drop the re-exports from `pages/Settings.tsx`. Shared
primitives they use (`Field`, `GlassCard`, `useSettingsData`…) are already
importable from `../../ui` / plugin-sdk, so this is file motion plus import
rewrites, no seam change.

### 2.3 Overview page hard-codes feature widgets — MEDIUM

`web/src/pages/Overview.tsx` renders links/mail/domains stat cards, the
quick-start checklist (`/links`, `/mail?tab=settings`, `/domains` navigation),
and the recent-emails panel. The stat cards already hide when their backend
field is absent (`o.links !== undefined` — the backend aggregates via service
lookups, so a missing plugin yields no field), but the quick-start steps and
their nav targets are unconditional: with the links plugin disabled the
checklist still sends the user to a 404-gated `/links`.

**Design**: add an `overview` seam to `UIPlugin` — a plugin contributes stat
cards and quick-start steps (`{ id, title, desc, path, completed(o) }`), and
`Overview.tsx` renders core steps (invite team) plus contributions from
enabled plugins only. Interim cheap fix: gate each hard-coded step on the same
`o.<field> !== undefined` check the cards use.

### 2.4 Commercial client surface in core `web/src/api.ts` — MEDIUM

`api.ts` still declares Pro-only types and methods with zero OSS consumers:
`Subscription`, `Transaction`, `FinanceSummary`, `Customer`, and the
`subscriptions`/`transactions`/`financeSummary` request helpers. The feature
plugins already own their API clients (`web/src/plugins/*/api.ts`), so these
belong in the corresponding octarq-pro plugin packages.

**Design**: delete them from core `api.ts`; octarq-pro plugins ship their own
typed clients (the `req` helper is exported / re-exportable via plugin-sdk).
Core keeps `Link`/`Mailbox`/`Domain` types only where a core surface (webhook
badges, overview) genuinely needs them — audit each at move time.

### 2.5 Buyer portal is a commercial app shipped by core — MEDIUM

`web/src/portal/` (732-line `PortalApp.tsx` + `portal.html` second Vite entry,
`i18n/pages/portal.ts`) is the buyer self-service portal (subscriptions,
license keys). The Go server unconditionally serves it at `/portal`
(`internal/server/server.go` `portalStatic`), but every API it calls is a Pro
plugin route — in an OSS build the portal renders and then 404s.

**Design**: move the portal app to octarq-pro. Needs one new seam: a plugin
must be able to register an embedded static frontend under a path prefix
(e.g. `ctx.HandleStatic(prefix string, fs fs.FS)` mirroring `HandleRoot`).
Core drops the `portal/` embed; the OSS `/portal` path 404s cleanly.

### 2.6 Pro naming baked into shell chrome — LOW

- `web/src/i18n/en.ts` / `zh.ts` `nav.*` translations for Pro menu items
  (`storefront`, `licenses`, `billing`, `finance`, `inbox-ai`, `vps`,
  `sshkeys`, `license`). Used by `t(\`nav.${id}\`, fallback)` — safe to remove
  once plugins can contribute `nav.*` keys (see 2.7); until then removing them
  only degrades zh labels for Pro builds.
- `web/src/shell/areas.tsx`: the `Commerce` area shell (empty group shells) and
  the commerce keyword routing in `areaForCategory`. `UIPlugin.areas` already
  exists — the Pro commerce plugin should declare its own area and the shell
  entry be deleted.
- `PersonalSettings` group label `Subscriptions` (`en.ts` line ~81) — verify
  owner and move with 2.4.

### 2.7 Plugin i18n cannot extend core namespaces — enabler for 2.6

Plugin `i18n` resources merge under the plugin's own namespace, so a plugin
cannot supply `nav.<id>` or `settings.pluginDesc.<key>` translations that
core-rendered chrome looks up. **Design**: let a UIPlugin's i18n object carry
reserved top-level keys (`nav`, `settings.pluginDesc`) that the SDK deep-merges
into the shared namespaces at registration (core wins on conflicts). That
unlocks deleting the Pro entries in 2.6 without losing zh menu labels.

## 3. Suggested sequencing

1. **2.1 + 2.2** (one PR): backend menus → plugins, settings pages → plugins.
   Pure decoupling, no new seams, biggest bang.
2. **2.3** interim gating fix immediately; the widget seam with whichever PR
   next touches Overview.
3. **2.7 → 2.6** (one PR): i18n merge seam, then delete Pro naming from core.
4. **2.4 + 2.5** (coordinated with octarq-pro): move client surface + portal;
   add the `HandleStatic` seam in core first so pro can pick it up.

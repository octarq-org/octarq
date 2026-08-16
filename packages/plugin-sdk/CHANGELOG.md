# @octarq-org/plugin-sdk

## 0.10.0

### Minor Changes

- 00f1ca0: Add `UIPlugin.replaces?: string[]` — the plugin-market "enhanced edition" seam:
  a plugin declares which other plugins it supersedes, and the registry excludes
  the replaced plugins wholesale (routes, widgets, areas, i18n all stop applying)
  so an enhanced edition can take over a feature from the OSS/vanilla plugin.

  Replacement is derived at read time from the full registry — never applied at
  registration — so the composed result is order-independent. Invalid
  declarations are composition errors surfaced like name collisions: two plugins
  replacing the same target throws in dev / console.errors in prod (neither
  wins), a `replaces` name matching no composed plugin warns in dev and is
  silently ignored in prod, self-replacement and replace chains are rejected.

## 0.9.6

### Patch Changes

- a992213: Surface `UIPlugin` name collisions instead of dropping the second registration
  in silence. A duplicate name now throws in development and logs an error in
  production, mirroring the backend's `preflightNameCollisions`, which refuses to
  start on duplicate plugin names.

  Silence was how the Pro audit page went unrendered since its first release: it
  shared a name with the core audit plugin, and the registry returned early
  without a word. A plugin name is an identity, and two plugins claiming one is a
  composition mistake the author needs to see.

## 0.9.5

### Patch Changes

- 86a6d8a: Add `useBrandRefresh()`, the host callback that re-reads and re-applies the
  operator's branding. A plugin that changes the branding can now tell the shell to
  catch up instead of leaving the sidebar mark, page title and accent colours on
  the old brand until a manual reload.

## 0.9.4

### Patch Changes

- f6b9107: Form primitives now take their focus ring, checked fill and info accent from
  the brand tokens instead of hardcoded indigo, so white-label branding reaches
  the settings pages.

## 0.9.3

### Patch Changes

- 6390377: Badge 合并为 SDK 唯一实现：修复 children 渲染丢失的回归，variant/tone 双 prop 均保留

## 0.9.2

### Patch Changes

- b85cdb7: 主按钮/焦点环/Pro 徽标改为读取品牌 CSS 变量

## 0.9.1

### Patch Changes

- 424ae69: Improve Dialog layout responsiveness on mobile viewports with bottom-sheet presentation.

## 0.9.0

### Minor Changes

- 435eeb4: Add `PasswordConfirmProvider` / `usePasswordConfirm` / `confirmPassword` — a
  shared password re-authentication dialog, mirroring the existing confirm dialog.
  It resolves the password the user typed (or `null` if dismissed) so a sensitive
  action can hand it straight to the server call that verifies it, instead of each
  page growing its own password field.

## 0.8.0

### Minor Changes

- 3c521b8: Add `confirmDialog()` / `useConfirm()` / `ConfirmProvider`, and remove the
  `UIPlugin.menu` field along with the `uiMenus()` helper.

  **New — `confirmDialog()`.** The imperative replacement for `window.confirm()`,
  mirroring how `toast` replaced `alert()`. Native `confirm()` renders unstyled OS
  chrome and blocks the event loop; this resolves a promise against the same
  dialog the rest of the dashboard uses. Mount `ConfirmProvider` once near the
  root — outside a provider the promise resolves `false`, so a missing provider
  denies rather than silently confirms.

  One hazard the native call did not have: this returns a promise, and a promise
  is truthy, so a forgotten `await` makes every guard pass. That is why the
  singleton is named `confirmDialog` and not `confirm` — `if (!confirm(...))`
  keeps working by accident, `if (!confirmDialog(...))` does not compile away
  quietly.

  **Breaking — `UIPlugin.menu` and `uiMenus()` are gone.** Sidebar placement now
  comes only from the Go plugin's `MenuProvider` (`Menus()` → `/api/menus`). The
  host already dropped any entry whose path the backend did not announce — that is
  the mechanism that makes a disabled feature's menu disappear — so a
  frontend-declared menu never stood alone; it was a hand-maintained copy, and the
  copies had drifted (one plugin's category read `Operations` on the Go side and
  `Marketing` on the frontend). Plugins that declared `menu` should delete it and
  declare the entry in their Go `Menus()`; the field's only remaining merit,
  painting the sidebar before the API answers, is handled by the host caching the
  last `/api/menus` response.

## 0.7.0

### Minor Changes

- 57caeef: Export OctarqMark and BrandGlyph components and ARCH_PATH from brand module.

## 0.6.0

### Minor Changes

- 12cfa13: Add `NotificationChannelFormContext` and `useNotificationChannelForm`.

  A plugin that registers a notification channel provider can now contribute its
  config form to `settings-notification-channel:<type>` and read/write the
  channel's config through the hook, instead of shipping a separate settings card
  of its own. Core's own telegram and webhook forms go through the same slot.

## 0.5.0

### Minor Changes

- 9d1b981: Publish accumulated i18n infrastructure: `Partial<Resources>` with English fallback (#54) plus Spanish, Portuguese, and Japanese locales and the `Lang`/`LANGS` additions (#54/#57). Needed by the Cloud dashboard for multi-language support.

## 0.4.0

### Minor Changes

- 9d42864: Add optional `groups?: string[]` to `UIArea`. A plugin-declared area can now
  carry ordered group shells, and a menu whose `category` matches one of those
  group labels routes into the area (in addition to matching the area's id or
  title). This lets a Pro edition own a whole multi-group area — e.g. Commerce
  with Sales/Billing/Finance — after the OSS core stopped shipping an empty shell
  and keyword routing for it.

## 0.3.0

### Minor Changes

- e5f4576: Add a toast notification system to the shared UI surface: `ToastProvider`,
  the `useToast()` hook, and an imperative `toast` singleton (`toast.success` /
  `toast.error` / `toast.info`). Non-blocking, glass-themed, `aria-live`
  announced — the intended replacement for native `alert()` in dashboards and
  plugins. Mount `<ToastProvider>` once at the app root; call `toast.*` (or
  `useToast()`) anywhere below it.

## 0.2.0

### Minor Changes

- 070d953: Make the SDK self-contained so plugins can ship as independent packages.

  - **i18n**: `I18nProvider` + `useTranslation`/`useI18n` (the host feeds resource
    dictionaries; the SDK folds in composed-plugin namespaces).
  - **brand**: `BrandProvider` + `useAppName` (the host feeds the product name).
  - **locked-state UI**: `LockedFeature` and `LockedFallback` move into the
    published package, driven by the SDK's own i18n + brand context instead of the
    host app's — so a plugin can render the 402/404 upsell without importing
    anything app-internal.

  The package root (`.`) now re-exports the full plugin surface (contract + ui +
  i18n + brand); the pure component set is still available under `./ui`.

## 0.1.0

### Minor Changes

- 54c0214: Initial release of the octarq frontend plugin SDK.

  Ships the `UIPlugin` contract and build-time registry (routes, menus, i18n,
  locked-state fallback) plus the shadcn/Base-UI-backed shared component library
  (cn, Button, Badge, GlassCard, Modal, Toggle, Field, Input, Textarea, Select,
  Tabs, Tooltip, Table, Skeleton, …) that plugin pages build against.

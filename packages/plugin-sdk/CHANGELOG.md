# @octarq-org/plugin-sdk

## 0.15.0

### Minor Changes

- 6be7d58: The presentational table parts are now part of the SDK: `TableEmpty`,
  `TableError`, `TableSkeleton` and `TablePagination`.

  These lived only in the host app's `pro-table`, so a plugin rendering a table had
  no way to reach them: ten Pro plugins and the `twofa` plugin all hand-roll raw
  `<table>` markup with no empty/loading/error state at all.

  Deliberately **not** the whole concern: `ProTable` itself is a data grid built on
  `@tanstack/react-table` + `react-query` + `zod`, and it stays app-side. Moving it
  would push three heavy dependencies into this package — which holds seven — and
  into consumer plugins that need none of it: none of the ten hand-rolled tables
  sorts, and not one of them uses a table library. These four parts depend on
  nothing beyond what the SDK already ships.

  - Icons are drawn inline, keeping the package icon-library-free (same reasoning
    as `LockedFeature`'s key glyph); `TableEmpty` accepts an `icon` override.
  - Copy resolves from the host dictionary (`proTable.*`) through the SDK's own
    i18n context — the host-fed contract `LockedFeature` and `FormError` already use.
  - `TableError` also drops an `as any` cast when reading an axios-style
    `{ response: { status } }`, narrowing it properly.

## 0.14.0

### Minor Changes

- 38e5bbc: Design tokens are now enumerable as data (`TOKENS`, `SEED_TOKENS`,
  `tokensByGroup`, `tokenByName`, `themeAlias`, `readTokenValues`).

  The token set previously existed only as CSS custom properties in the host
  stylesheet, so nothing could enumerate it: picking the right token meant reading
  `styles.css`, and building any kind of token inspector meant scraping it or
  reading computed styles off the DOM.

  The inventory carries **names, groups and roles — not values**:

  - `TokenDef.root` / `.dark` / `.themed` record which of the three blocks
    (`:root`, `.dark`, `@theme inline`) declares the token, so a consumer can tell
    theme-invariant tokens from ones with a dark override.
  - `seed` marks the three literals a white-label rebrand overrides — the entire
    rebrand write surface; everything brand-tinted derives from `--primary`.
  - `readTokenValues()` resolves live values from the DOM, which is the only source
    that reflects a runtime rebrand and the active light/dark mode.

  Values are deliberately not copied here: a duplicated brand hex is what the
  colour lint forbids, and a static copy would go stale against a rebrand. The set
  is held in lockstep with the stylesheet by a fail-closed parity test in the host
  (`web/src/test/tokensParity.test.ts`) asserting set equality in both directions
  for all three blocks.

## 0.13.0

### Minor Changes

- 70f4560: Code, Guide and timeAgo are now part of the SDK's UI surface.

  These were the last three app-coupled extras the host's facade had to union in
  (`web/src/plugin-sdk/ui.tsx`), which is why a published plugin package could not
  use them — a Pro plugin rendering a copy-to-clipboard value or a collapsible
  guidance panel had no equivalent to reach for.

  All three turned out to be pure:

  - `Code` / `Guide` only needed `useTranslation`, which the SDK already owns and
    the host already feeds the `uiCommon.*` dictionary into.
  - `timeAgo` has no dependencies at all.

  Promoting them lets the host's facade shrink to a single hop, so the SDK is now
  the only UI surface in the product rather than one of two.

  `timeAgo` keeps its existing English-only strings: it is a plain function with no
  i18n context to read, so localizing it is a `useTimeAgo()` hook (a change to
  every call site), not a move. That is deliberately left for a separate change.

## 0.12.0

### Minor Changes

- 00bd8f9: Alert, FormError and RouteFallback are now part of the SDK's UI surface.

  All three lived only in the host app (`web/src/components/ui/`), so a plugin
  package could not render them: a Pro plugin showing an inline status message had
  to approximate one with a Badge, and a plugin's own `<Suspense>` boundary had no
  spinner to fall back on.

  - `Alert` (+ `alertVariants`, `AlertProps`) — tones read the `--{info,success,warning,danger}-*`
    tokens, so no literal brand hue is involved.
  - `FormError` (+ `formErrorMessage`, `formErrorStatusKeys`) — reads the SDK's own
    i18n context; the `uiCommon.*` keys it resolves are supplied by the host
    dictionary, exactly as `LockedFeature` already does.
  - `RouteFallback` — the shared route-chunk spinner.

  Removing the app-local copies also removes the second `cn` implementation
  (`web/src/lib/utils.ts`), which existed only to serve them.

- 67a7e67: Button is now the single Button definition for the whole product.

  It previously existed twice: this package shipped a gradient (indigo→violet)
  `primary`, while the host app carried a flat one in
  `web/src/components/ui/Button.tsx` that its barrel re-exported _in place of_
  this one — an explicit named export beats `export *`. Core pages therefore
  rendered the flat button, every plugin rendered the gradient one, and `size`
  and `secondary` existed on only one of them.

  - `primary` is now FLAT (`bg-primary` / `hover:bg-primary-hover` / `text-primary-foreground`).
    A gradient end is a hardcoded hue that cannot follow the `--primary` seed, so
    it stayed indigo on a white-label rebranded instance.
  - New `size` axis: `sm | md | lg` (default `md`), exported as `ButtonSize`.
  - New `secondary` variant: `bg-muted` + `border-border`.
  - `danger` derives from the `--danger-*` tokens instead of a literal `rose`.
  - `Button` forwards its ref to the underlying `<button>`.
  - `ButtonProps` is now exported as an interface (was inline).

  Visual change for consumers on the previous gradient primary: it is flat now.

## 0.11.0

### Minor Changes

- 8995bec: Add `UIPlugin.instanceRoutes?: UIRoute[]` and `uiInstanceRoutes()` — the
  frontend half of the instance-scope plugin seam. Routes registered here render
  under the `/instance` console basename instead of `/admin`, paired with the Go
  `plugin.InstanceMenuProvider` interface whose entries the instance-admin-gated
  `GET /api/instance/menus` endpoint serves. The console renders a plugin rail
  entry only when the backend announces it AND the frontend registers a route for
  the same path, mirroring the tenant sidebar's trust in `/api/menus`.

  `instanceRoutes` goes through the same `replaces` filtering as `routes`: a
  replaced plugin's instance pages disappear with it, so a superseded plugin can
  never keep a page in the instance console.

## 0.10.1

### Patch Changes

- f0c1aa5: Fix the `LockedFeature` upgrade button's navigation target: it pointed at
  `/admin/license`, which only worked because the host app happened to redirect
  `/license` to `/settings/license` — a fragile coupling that would strand the
  paywall button on "Not part of this build" if that redirect were ever removed.
  Point it directly at the license-activation page (`/admin/settings/license`,
  i.e. router path `/settings/license` under the app's `/admin` basename), the
  path the Pro licensing plugin actually registers.

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

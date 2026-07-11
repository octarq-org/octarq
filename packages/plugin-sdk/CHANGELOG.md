# @octarq-org/plugin-sdk

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

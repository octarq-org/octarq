---
"@octarq-org/plugin-sdk": minor
---

Make the SDK self-contained so plugins can ship as independent packages.

- **i18n**: `I18nProvider` + `useTranslation`/`useI18n` (the host feeds resource
  dictionaries; the SDK folds in composed-plugin namespaces).
- **brand**: `BrandProvider` + `useAppName` (the host feeds the product name).
- **locked-state UI**: `LockedFeature` and `LockedFallback` move into the
  published package, driven by the SDK's own i18n + brand context instead of the
  host app's — so a plugin can render the 402/404 upsell without importing
  anything app-internal.

The package root (`.`) now re-exports the full plugin surface (contract + ui +
i18n + brand); the pure component set is still available under `./ui`.

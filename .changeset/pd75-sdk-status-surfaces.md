---
"@octarq/plugin-sdk": minor
---

Alert, FormError and RouteFallback are now part of the SDK's UI surface.

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

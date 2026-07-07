// @led/plugin-sdk — the frontend plugin SDK barrel.
//
// A frontend plugin imports everything it needs from this one module:
//   - the `UIPlugin` contract and helper types,
//   - the shared UI surface (GlassCard, PageHeader, LockedFallback, …),
//   - `useTranslation` for i18n.
// The app imports the registry readers to compose plugins in.
//
// Today this resolves to `web/src/plugin-sdk` via the `@led/plugin-sdk` alias
// (vite.config.ts + tsconfig paths). Extracting it to a published npm package
// later means moving this folder out and pointing the alias at the package —
// no import churn in plugin code, because plugins already import by the
// `@led/plugin-sdk` name.
export type {
  UIPlugin,
  UIRoute,
  PluginMenuItem,
  PluginI18n,
  LazyPage,
  LockedFallback as LockedFallbackType,
} from "./types";

export {
  registerUIPlugin,
  uiPlugins,
  uiRoutes,
  uiMenus,
  uiPluginI18n,
  resetRegistry,
} from "./registry";

export * from "./ui";

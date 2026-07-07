// The `@led/plugin-sdk` contract sub-barrel: the pure, app-independent plugin
// contract and its registry. Re-exported by the package root (`src/index.ts`).
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

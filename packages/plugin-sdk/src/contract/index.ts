// The `@octarq/plugin-sdk` contract sub-barrel: the pure, app-independent plugin
// contract and its registry. Re-exported by the package root (`src/index.ts`).
export type {
  UIPlugin,
  UIRoute,
  UIWidget,
  UIArea,
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
  uiWidgets,
  uiAreas,
  uiPluginI18n,
  uiPluginSharedI18n,
  resetRegistry,
} from "./registry";

export { ExtensionSlot } from "./ExtensionSlot";

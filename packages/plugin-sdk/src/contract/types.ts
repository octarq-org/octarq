// The frontend plugin contract — the UI analog of the Go `plugin.Plugin`
// interface. A commercial or third-party plugin ships a module conforming to
// `UIPlugin`; the app composes it into the route/menu/i18n registry AT BUILD
// TIME (see the app's plugins/index.ts). This mirrors the backend seam: Go code
// implements `plugin.Plugin`, JS code implements `UIPlugin`, and both are
// composed without forking octarq.
//
// This module deliberately imports nothing app-internal — it is the published
// public contract of `@octarq-org/plugin-sdk`.
import type { ComponentType, LazyExoticComponent } from "react";

// A lazily-loaded page component. Plugins wrap their page in `React.lazy` so the
// heavy page module lands in its own async chunk that only loads when the route
// is actually visited — and, in a build that never composes the plugin in, is
// never referenced at all.
export type LazyPage = LazyExoticComponent<ComponentType<Record<string, never>>>;

// A route contributed by a plugin. `path` is an absolute admin path (e.g.
// "/licenses"); it is rendered under the same `/admin` basename as core routes.
export interface UIRoute {
  path: string;
  Component: LazyPage;
}

// A sidebar entry contributed by a plugin. Shape matches the backend
// `MenuItem` (api.MenuItem) so plugin menus flow through the exact same
// area-placement logic (`areaForCategory`) as dynamic backend menus — no
// parallel mechanism. `category` picks the top-level area; `icon` is an emoji
// or icon key rendered by the sidebar.
export interface PluginMenuItem {
  id: string;
  label: string;
  path: string;
  icon: string;
  category: string;
}

// A pair of per-language namespace objects a plugin injects into i18n. The key
// under which they merge is the plugin's `name`, so a plugin owns the
// `"<name>.*"` translation namespace (e.g. licenses page keys live under
// `licenses.*`). Missing in a build that doesn't compose the plugin — which is
// fine, because that build never renders the plugin's page.
export interface PluginI18n {
  en: Record<string, unknown>;
  zh: Record<string, unknown>;
}

// The component a plugin renders for the gated 402 (unlicensed) / 404 (plugin
// not in this build) states — the app ships `LockedFallback` for this. Kept in
// the contract so the app's plugin-route boundary can degrade to it if a page
// chunk fails to load.
export type LockedFallback = ComponentType<{ status: number }>;

// A frontend plugin: the unit the registry composes. `name` is the stable id
// and should match the Go `plugin.Plugin.Name()` of its backend counterpart so
// the two halves of a feature are traceable to each other.
export interface UIPlugin {
  name: string;
  routes: UIRoute[];
  menu?: PluginMenuItem[];
  i18n?: PluginI18n;
  lockedFallback?: LockedFallback;
}

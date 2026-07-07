// The frontend plugin registry — the runtime seam the app reads to discover
// composed plugins. It is populated exactly once, at module-eval time, by the
// build-time injection module (the app's plugins/index.ts): the OSS build
// registers nothing (registry stays empty ⇒ Pro routes 404-degrade), a
// commercial build registers real plugin modules.
//
// This is the JS mirror of `app.App.Use(plugin.Plugin)` on the Go side.
import type { PluginI18n, PluginMenuItem, UIPlugin, UIRoute } from "./types";

const REGISTRY: UIPlugin[] = [];

// Compose a plugin into the app. Called by the injection module at build time.
// Idempotent per plugin name so a double-import can't duplicate routes.
export function registerUIPlugin(plugin: UIPlugin): void {
  if (REGISTRY.some((p) => p.name === plugin.name)) return;
  REGISTRY.push(plugin);
}

// All composed plugins, in registration order.
export function uiPlugins(): UIPlugin[] {
  return REGISTRY;
}

// Every plugin route, flattened — the app maps these into <Routes>.
export function uiRoutes(): UIRoute[] {
  return REGISTRY.flatMap((p) => p.routes);
}

// Every plugin sidebar entry, flattened — folded into the sidebar alongside
// dynamic backend menus and placed by the shared `areaForCategory`.
export function uiMenus(): PluginMenuItem[] {
  return REGISTRY.flatMap((p) => p.menu ?? []);
}

// Merged plugin i18n namespaces, keyed by plugin name, per language. The
// I18nProvider spreads these over the core resources at render time (order-
// independent of module eval).
export function uiPluginI18n(): PluginI18n {
  const en: Record<string, unknown> = {};
  const zh: Record<string, unknown> = {};
  for (const p of REGISTRY) {
    if (!p.i18n) continue;
    en[p.name] = p.i18n.en;
    zh[p.name] = p.i18n.zh;
  }
  return { en, zh };
}

// Test-only: clear the registry between cases. Not used by the app.
export function resetRegistry(): void {
  REGISTRY.length = 0;
}

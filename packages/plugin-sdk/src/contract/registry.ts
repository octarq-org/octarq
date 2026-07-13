// The frontend plugin registry — the runtime seam the app reads to discover
// composed plugins. It is populated exactly once, at module-eval time, by the
// build-time injection modules (the app's plugins/core for the always-composed
// core-feature plugins, then the manifest-generated `#octarq-plugins` module):
// every edition registers its core plugins; only a manifest that names a Pro
// plugin registers it too (absent ⇒ its routes 404-degrade).
//
// This is the JS mirror of `app.App.Use(plugin.Plugin)` on the Go side.
import type { PluginI18n, PluginMenuItem, UIArea, UIPlugin, UIRoute, UIWidget } from "./types";

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

// Every widget registered for `slot`, across all plugins, in ascending `order`
// (missing order sorts as 0; ties keep registration order — Array.sort is
// stable). Rendered by <ExtensionSlot name={slot}/>. Empty registry ⇒ empty
// array ⇒ the slot renders nothing (the OSS build).
export function uiWidgets(slot: string): UIWidget[] {
  return REGISTRY.flatMap((p) => p.widgets ?? [])
    .filter((w) => w.slot === slot)
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
}

// Every NEW top-level area contributed by plugins, flattened and deduped by id
// (first registration wins, matching registerUIPlugin's idempotence). The app
// appends these to its static areas and routes menus into them through the
// shared `areaForCategory` pipeline.
export function uiAreas(): UIArea[] {
  const seen = new Set<string>();
  return REGISTRY.flatMap((p) => p.areas ?? []).filter((a) => {
    if (seen.has(a.id)) return false;
    seen.add(a.id);
    return true;
  });
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

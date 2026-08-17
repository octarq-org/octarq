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

// The plugins a read sees: REGISTRY minus every plugin that another plugin's
// `replaces` names. Derived lazily from the FULL registry at first read (and
// invalidated on every mutation), never applied at registration time — so the
// composed result is independent of registration order. See UIPlugin.replaces
// in types.ts for the contract-level statement.
let derivedPlugins: UIPlugin[] | null = null;

// Composition errors are surfaced exactly like name collisions: throw in dev,
// console.error in prod (a bad declaration must not white-screen the admin,
// but must stay visible).
function reportCompositionError(message: string): void {
  if (import.meta.env.DEV) {
    throw new Error(message);
  }
  console.error(message);
}

function effectivePlugins(): UIPlugin[] {
  if (derivedPlugins !== null) return derivedPlugins;

  const replaced = new Set<string>();
  const replacersByTarget = new Map<string, UIPlugin[]>();

  for (const p of REGISTRY) {
    const targets = new Set(p.replaces ?? []);
    for (const target of targets) {
      if (target === p.name) {
        reportCompositionError(
          `UIPlugin replaces: "${p.name}" declares replaces: ["${target}"] — ` +
            `a plugin cannot replace itself. Remove the self-reference.`,
        );
        continue;
      }
      const replacers = replacersByTarget.get(target);
      if (replacers) {
        replacers.push(p);
      } else {
        replacersByTarget.set(target, [p]);
      }
    }
  }

  for (const [target, replacers] of replacersByTarget) {
    if (replacers.length > 1) {
      reportCompositionError(
        `UIPlugin replaces ambiguity: "${target}" is declared as replaced by ` +
          `${replacers.map((r) => `"${r.name}"`).join(" and ")}. A replacement target ` +
          `must have exactly one replacer — rename or drop one of the two declarations.`,
      );
      continue; // no winner: the target stays composed
    }
    const targetPlugin = REGISTRY.find((p) => p.name === target);
    if (!targetPlugin) {
      if (import.meta.env.DEV) {
        console.warn(
          `UIPlugin replaces: "${replacers[0].name}" declares replaces: ["${target}"], ` +
            `but no composed plugin is named "${target}" — a typo, or an optional ` +
            `dependency that this build doesn't compose. The declaration is ignored.`,
        );
      }
      continue;
    }
    if (targetPlugin.replaces?.length) {
      reportCompositionError(
        `UIPlugin replace chain is not supported: "${replacers[0].name}" replaces ` +
          `"${target}", but "${target}" itself declares replaces: ` +
          `${targetPlugin.replaces.map((n) => `"${n}"`).join(", ")}. Chain declarations ` +
          `are a composition error — declare each level explicitly or drop one.`,
      );
      continue;
    }
    replaced.add(target);
  }

  derivedPlugins = REGISTRY.filter((p) => !replaced.has(p.name));
  return derivedPlugins;
}

// Compose a plugin into the app. Called by the injection module at build time.
// Idempotent per plugin name so a double-import can't duplicate routes.
//
// A name collision is never silent — it is a composition mistake (two editions
// claiming the same plugin id, the exact bug that kept the Pro audit page from
// ever rendering). Mirroring the backend's preflightNameCollisions, which
// refuses startup on duplicate plugin names:
//   - dev  → throw, so the mistake fails loudly in front of the developer;
//   - prod → console.error + first-wins, so a third-party plugin with a clashing
//     name can't white-screen the whole admin, but the conflict stays visible.
export function registerUIPlugin(plugin: UIPlugin): void {
  const existing = REGISTRY.find((p) => p.name === plugin.name);
  if (existing) {
    const message =
      `UIPlugin name collision: "${existing.name}" is already registered by another plugin ` +
      `(routes: ${existing.routes.length}); the incoming plugin "${plugin.name}" ` +
      `(routes: ${plugin.routes.length}) was ignored. A plugin name is its identity — ` +
      `first registration wins, so rename one of the two plugins.`;
    if (import.meta.env.DEV) {
      throw new Error(message);
    }
    console.error(message);
    return;
  }
  REGISTRY.push(plugin);
  derivedPlugins = null;
}

// All composed plugins, in registration order, minus any that another plugin
// replaces.
export function uiPlugins(): UIPlugin[] {
  return effectivePlugins();
}

// Every plugin route, flattened — the app maps these into <Routes>.
export function uiRoutes(): UIRoute[] {
  return effectivePlugins().flatMap((p) => p.routes);
}

// Every plugin instance-route, flattened — the instance console maps these
// into its <Routes> under the /instance basename. Same `replaces` filter as
// uiRoutes: a replaced plugin's instance pages disappear with it, so a
// superseded plugin can never keep a page in the instance console.
export function uiInstanceRoutes(): UIRoute[] {
  return effectivePlugins().flatMap((p) => p.instanceRoutes ?? []);
}

// Every widget registered for `slot`, across all plugins, in ascending `order`
// (missing order sorts as 0; ties keep registration order — Array.sort is
// stable). Rendered by <ExtensionSlot name={slot}/>. Empty registry ⇒ empty
// array ⇒ the slot renders nothing (the OSS build).
export function uiWidgets(slot: string): UIWidget[] {
  return effectivePlugins()
    .flatMap((p) =>
      (p.widgets ?? []).map((w) => ({ ...w, pluginName: w.pluginName ?? p.name }))
    )
    .filter((w) => w.slot === slot)
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
}

// Every NEW top-level area contributed by plugins, flattened and deduped by id
// (first registration wins, matching registerUIPlugin's idempotence). The app
// appends these to its static areas and routes menus into them through the
// shared `areaForCategory` pipeline.
export function uiAreas(): UIArea[] {
  const seen = new Set<string>();
  return effectivePlugins().flatMap((p) => p.areas ?? []).filter((a) => {
    if (seen.has(a.id)) return false;
    seen.add(a.id);
    return true;
  });
}

// Merged plugin i18n namespaces, keyed by plugin name, per language. The
// I18nProvider spreads these over the core resources at render time (order-
// independent of module eval). The reserved `_shared` key is NOT a plugin
// namespace — it is collected separately by uiPluginSharedI18n.
export function uiPluginI18n(): PluginI18n {
  const out: Record<string, Record<string, unknown>> = {};
  for (const p of effectivePlugins()) {
    if (!p.i18n) continue;
    for (const [lang, dict] of Object.entries(p.i18n)) {
      if (!dict) continue;
      const { _shared: _sharedNs, ...ns } = dict as Record<string, unknown>;
      if (!out[lang]) out[lang] = {};
      out[lang][p.name] = ns;
    }
  }
  return out;
}

// Recursively fold `extra` into `base` (both plain objects); `base` wins on
// leaf conflicts so an earlier registration — and ultimately the core
// resources layered on top — can never be overridden by a plugin.
function mergeUnder(base: Record<string, unknown>, extra: unknown): void {
  if (extra == null || typeof extra !== "object") return;
  for (const [k, v] of Object.entries(extra as Record<string, unknown>)) {
    const cur = base[k];
    if (cur != null && typeof cur === "object" && v != null && typeof v === "object") {
      mergeUnder(cur as Record<string, unknown>, v);
    } else if (!(k in base)) {
      base[k] = v;
    }
  }
}

// The deep-merged `_shared` contributions of every composed plugin: shared-
// namespace translations (e.g. `nav.<menu id>`, `settings.pluginDesc.<key>`)
// that core-rendered chrome looks up. The I18nProvider layers core resources
// OVER this dict, so core copy always wins.
export function uiPluginSharedI18n(): PluginI18n {
  const out: Record<string, Record<string, unknown>> = {};
  for (const p of effectivePlugins()) {
    if (!p.i18n) continue;
    for (const [lang, dict] of Object.entries(p.i18n)) {
      if (!dict) continue;
      if (!out[lang]) out[lang] = {};
      mergeUnder(out[lang], (dict as Record<string, unknown>)._shared);
    }
  }
  return out;
}

// Test-only: clear the registry between cases. Not used by the app.
export function resetRegistry(): void {
  REGISTRY.length = 0;
  derivedPlugins = null;
}

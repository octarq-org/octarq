import type { PluginI18n, PluginMenuItem, UIArea, UIPlugin, UIRoute, UIWidget } from "./types";

const REGISTRY: UIPlugin[] = [];

let derivedPlugins: UIPlugin[] | null = null;

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
      continue;
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

export function uiPlugins(): UIPlugin[] {
  return effectivePlugins();
}

export function uiRoutes(): UIRoute[] {
  return effectivePlugins().flatMap((p) => p.routes);
}

export function uiInstanceRoutes(): UIRoute[] {
  return effectivePlugins().flatMap((p) => p.instanceRoutes ?? []);
}

export function uiWidgets(slot: string): UIWidget[] {
  return effectivePlugins()
    .flatMap((p) =>
      (p.widgets ?? []).map((w) => ({ ...w, pluginName: w.pluginName ?? p.name }))
    )
    .filter((w) => w.slot === slot)
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
}

export function uiOnboarding(): UIPlugin["onboarding"] | null {
  return effectivePlugins().find((p) => p.onboarding)?.onboarding ?? null;
}

export function uiAreas(): UIArea[] {
  const seen = new Set<string>();
  return effectivePlugins().flatMap((p) => p.areas ?? []).filter((a) => {
    if (seen.has(a.id)) return false;
    seen.add(a.id);
    return true;
  });
}

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

export function resetRegistry(): void {
  REGISTRY.length = 0;
  derivedPlugins = null;
}

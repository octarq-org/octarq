import type {
  ArtifactEditorRegistration,
  PluginI18n,
  PluginMenuItem,
  ToolCardRegistration,
  UIArea,
  UIPlugin,
  UIRoute,
  UIWidget,
} from "./types";

const REGISTRY: UIPlugin[] = [];

let derivedPlugins: UIPlugin[] | null = null;
let derivedCards: ToolCardRegistration[] | null = null;
let derivedArtifacts: ArtifactEditorRegistration[] | null = null;

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
  derivedCards = null;
  derivedArtifacts = null;
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

function validateCardMatch(match: string, pluginName: string): void {
  const stars = match.split("*").length - 1;
  if (stars > 0 && (stars > 1 || !match.endsWith("*"))) {
    reportCompositionError(
      `Tool card match "${match}" from plugin "${pluginName}" is invalid: ` +
        `use an exact tool name or one trailing "*" wildcard.`,
    );
  }
}

function cardEntries(): ToolCardRegistration[] {
  if (derivedCards !== null) return derivedCards;

  const out: ToolCardRegistration[] = [];
  const owners = new Map<string, string>();

  for (const p of effectivePlugins()) {
    for (const card of p.cards ?? []) {
      validateCardMatch(card.match, p.name);
      const owner = owners.get(card.match);
      if (owner !== undefined) {
        reportCompositionError(
          `Tool card match collision: "${card.match}" is registered by "${owner}" and ` +
            `"${p.name}". A tool card match must be unique across composed plugins.`,
        );
        continue;
      }
      owners.set(card.match, p.name);
      out.push(card);
    }
  }

  derivedCards = out;
  return out;
}

function artifactEntries(): ArtifactEditorRegistration[] {
  if (derivedArtifacts !== null) return derivedArtifacts;

  const out: ArtifactEditorRegistration[] = [];
  const owners = new Map<string, string>();

  for (const p of effectivePlugins()) {
    for (const artifact of p.artifacts ?? []) {
      if (!artifact.type.startsWith(`${p.name}.`)) {
        reportCompositionError(
          `Artifact editor type "${artifact.type}" from plugin "${p.name}" must start ` +
            `with "${p.name}.". An artifact type is namespaced by its owning plugin.`,
        );
        continue;
      }
      const owner = owners.get(artifact.type);
      if (owner !== undefined) {
        reportCompositionError(
          `Artifact editor type collision: "${artifact.type}" is registered by "${owner}" ` +
            `and "${p.name}".`,
        );
        continue;
      }
      owners.set(artifact.type, p.name);
      out.push(artifact);
    }
  }

  derivedArtifacts = out;
  return out;
}

export function uiToolCards(): ToolCardRegistration[] {
  return cardEntries();
}

export function uiArtifactEditors(): ArtifactEditorRegistration[] {
  return artifactEntries();
}

export function resolveToolCard(
  toolName: string,
): ToolCardRegistration["Component"] | null {
  const cards = cardEntries();

  for (const card of cards) {
    if (!card.match.includes("*") && card.match === toolName) {
      return card.Component;
    }
  }

  let best: ToolCardRegistration | null = null;
  let bestPrefix = -1;
  for (const card of cards) {
    if (!card.match.endsWith("*")) continue;
    const prefix = card.match.slice(0, -1);
    if (!toolName.startsWith(prefix)) continue;
    if (prefix.length > bestPrefix) {
      best = card;
      bestPrefix = prefix.length;
    }
  }

  return best?.Component ?? null;
}

export function resolveArtifactEditor(
  type: string,
): ArtifactEditorRegistration["Component"] | null {
  return artifactEntries().find((a) => a.type === type)?.Component ?? null;
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
  derivedCards = null;
  derivedArtifacts = null;
}

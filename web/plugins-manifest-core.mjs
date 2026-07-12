// Shared, framework-free logic for the manifest-driven plugin system. Both the
// Vite virtual-module plugin (plugins-manifest.ts) and the build-time installer
// (scripts/install-manifest-plugins.mjs) import this, so it is the single source
// of truth for "what a manifest entry means" — how it's resolved, imported, and
// installed. Keep it dependency-free ESM so a plain `node` invocation can load it.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

export const WEB_ROOT = dirname(fileURLToPath(import.meta.url));

// Resolve the active manifest entries, highest precedence first:
//   1. OCTARQ_PLUGINS          — inline JSON array of entries, or a comma-separated
//                                list of package specifiers (dynamic CI injection).
//   2. OCTARQ_PLUGINS_MANIFEST — path to a JSON manifest file (octarq-pro points here).
//   3. web/octarq.plugins.json — the committed default (OSS edition: example plugin).
export function resolveEntries() {
  const inline = process.env.OCTARQ_PLUGINS?.trim();
  if (inline) {
    if (inline.startsWith("[")) return JSON.parse(inline);
    return inline.split(",").map((s) => s.trim()).filter(Boolean);
  }
  const file = process.env.OCTARQ_PLUGINS_MANIFEST
    ? resolve(process.env.OCTARQ_PLUGINS_MANIFEST)
    : resolve(WEB_ROOT, "octarq.plugins.json");
  const parsed = JSON.parse(readFileSync(file, "utf8"));
  return parsed.plugins ?? [];
}

// Split a package specifier into name + optional semver range, honoring scoped
// names (the leading '@' of "@scope/pkg" is not a version separator).
//   "@scope/pkg@^1.2.0" → { name: "@scope/pkg", range: "^1.2.0" }
//   "pkg"              → { name: "pkg", range: "" }
export function splitSpec(spec) {
  const at = spec.lastIndexOf("@");
  if (at > 0) return { name: spec.slice(0, at), range: spec.slice(at + 1) };
  return { name: spec, range: "" };
}

// Normalize a manifest entry into the pieces both consumers need.
//   isLocal:     a relative-path bridge (resolved against the web root) — never installed.
//   importSpec:  what the generated module imports (absolute path for local, bare
//                package NAME — version stripped — for a package).
//   installSpec: what the installer hands to `pnpm add` (name[@range]), or null for local.
//   named:       a specific export to import, else the package's default export.
// A version may be given inline ("@scope/pkg@^1") or, for the object form, via a
// dedicated `version` field (which wins) so third-party manifests can pin plugins.
export function parseEntry(entry) {
  const raw = typeof entry === "string" ? entry : entry.from;
  const named = typeof entry === "string" ? undefined : entry.import;
  const version = typeof entry === "object" ? entry.version : undefined;
  if (raw.startsWith(".") || raw.startsWith("/")) {
    return { isLocal: true, importSpec: resolve(WEB_ROOT, raw), installSpec: null, named };
  }
  const { name, range } = splitSpec(raw);
  const wanted = version || range;
  return { isLocal: false, importSpec: name, installSpec: wanted ? `${name}@${wanted}` : name, named };
}

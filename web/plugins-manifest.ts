// Manifest-driven frontend plugin composition.
//
// The app imports the virtual module `#octarq-plugins` purely for its side
// effects (see web/src/main.tsx: `import "#octarq-plugins"`). This Vite plugin
// generates that module at build time from a *manifest* — a list of plugin
// packages to compose into this build. It replaces the old two-file seam
// (index.ts / index.pro.ts) and the VITE_OCTARQ_PLUGINS switch: WHICH plugins a
// build ships is now data, not code, so an edition is chosen by pointing at a
// different manifest instead of editing the source.
//
// The generated module is the frontend analog of a octarq-pro binary calling
// `app.App.Use(...)` for each Go plugin — composition still happens at build
// time, entirely outside the OSS core.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import type { Plugin } from "vite";

const WEB_ROOT = dirname(fileURLToPath(import.meta.url));

const VIRTUAL_ID = "#octarq-plugins";
const RESOLVED_ID = "\0octarq-plugins";

// A manifest entry is either a bare module specifier (the package's *default*
// export is the UIPlugin, or an array of them) or an object naming a specific
// export. The object form is mainly a migration bridge: it lets a build compose
// a plugin whose source still lives locally (`from: "./src/plugins/vps"`,
// resolved relative to the web root) or a package that only has a named export,
// before it becomes a standalone default-exporting package.
type ManifestEntry = string | { from: string; import?: string };
type Manifest = { plugins?: ManifestEntry[] };

// Resolve the active manifest, highest precedence first:
//   1. OCTARQ_PLUGINS          — an inline JSON array of entries (or a simple
//                                comma-separated list of package specifiers).
//                                For dynamic CI injection with no file to edit.
//   2. OCTARQ_PLUGINS_MANIFEST — a path to a JSON manifest file. For a repo that
//                                ships its own edition (octarq-pro points here).
//   3. web/octarq.plugins.json — the committed default (the OSS edition: it
//                                lists the example plugin).
function resolveEntries(): ManifestEntry[] {
  const inline = process.env.OCTARQ_PLUGINS?.trim();
  if (inline) {
    if (inline.startsWith("[")) return JSON.parse(inline) as ManifestEntry[];
    return inline.split(",").map((s) => s.trim()).filter(Boolean);
  }
  const file = process.env.OCTARQ_PLUGINS_MANIFEST
    ? resolve(process.env.OCTARQ_PLUGINS_MANIFEST)
    : resolve(WEB_ROOT, "octarq.plugins.json");
  const parsed = JSON.parse(readFileSync(file, "utf8")) as Manifest;
  return parsed.plugins ?? [];
}

// Resolve a specifier: relative paths (a local, not-yet-packaged plugin) are
// made absolute against the web root so they resolve from the virtual module,
// which has no real path of its own; package specifiers pass through untouched.
function resolveSpec(spec: string): string {
  return spec.startsWith(".") ? resolve(WEB_ROOT, spec) : spec;
}

function generate(entries: ManifestEntry[]): string {
  const lines = [
    "// AUTO-GENERATED from the plugin manifest — do not edit.",
    'import { registerUIPlugin } from "@octarq-org/plugin-sdk";',
  ];
  const locals: string[] = [];
  entries.forEach((entry, i) => {
    const spec = typeof entry === "string" ? entry : entry.from;
    const named = typeof entry === "string" ? undefined : entry.import;
    const local = `__p${i}`;
    const from = JSON.stringify(resolveSpec(spec));
    lines.push(named ? `import { ${named} as ${local} } from ${from};` : `import ${local} from ${from};`);
    locals.push(local);
  });
  // A package may export a single UIPlugin or an array of them (a group of
  // related plugins, e.g. a page plugin plus a route-less i18n-only plugin), so
  // flatten before registering.
  lines.push(`for (const p of [${locals.join(", ")}].flat()) registerUIPlugin(p);`);
  return lines.join("\n") + "\n";
}

export function octarqPlugins(): Plugin {
  return {
    name: "octarq-plugins-manifest",
    resolveId(id) {
      if (id === VIRTUAL_ID) return RESOLVED_ID;
    },
    load(id) {
      if (id === RESOLVED_ID) return generate(resolveEntries());
    },
  };
}

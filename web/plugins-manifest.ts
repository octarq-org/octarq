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
// The manifest is the single source of truth: packages it lists that aren't
// already in node_modules are fetched by scripts/install-manifest-plugins.mjs
// (run before the Vite build), so a build can compose third-party plugins by
// manifest alone — without adding them to this package's package.json. A package
// that still isn't resolvable here (e.g. a private plugin the build has no token
// for) is skipped with a warning, mirroring optionalDependencies' "excluded if
// it can't be installed" degradation; the corresponding page then 404s gracefully.
//
// The generated module is the frontend analog of an octarq-pro binary calling
// `app.App.Use(...)` for each Go plugin — composition still happens at build
// time, entirely outside the OSS core.
import { existsSync } from "node:fs";
import { join } from "node:path";
import type { Plugin } from "vite";
import { resolveEntries, parseEntry, WEB_ROOT } from "./plugins-manifest-core.mjs";

const VIRTUAL_ID = "#octarq-plugins";
const RESOLVED_ID = "\0octarq-plugins";

// Is a bare package name present in node_modules? Checks for the package's
// package.json rather than require.resolve so packages with an `exports` map
// (which can make require.resolve throw even when installed) aren't misjudged.
function installed(name: string): boolean {
  return existsSync(join(WEB_ROOT, "node_modules", ...name.split("/"), "package.json"));
}

function generate(entries: ReturnType<typeof resolveEntries>): string {
  const lines = [
    "// AUTO-GENERATED from the plugin manifest — do not edit.",
    'import { registerUIPlugin } from "@octarq-org/plugin-sdk";',
  ];
  const locals: string[] = [];
  entries.forEach((entry, i) => {
    const { isLocal, importSpec, named } = parseEntry(entry);
    if (!isLocal && !installed(importSpec)) {
      console.warn(`[octarq-plugins] skipping "${importSpec}" — not installed; its page will 404-degrade`);
      return;
    }
    const local = `__p${i}`;
    const from = JSON.stringify(importSpec);
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

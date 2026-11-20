import { existsSync } from "node:fs";
import { join } from "node:path";
import type { Plugin } from "vite";
import { resolveEntries, parseEntry, WEB_ROOT } from "./plugins-manifest-core.mjs";

const VIRTUAL_ID = "#octarq-plugins";
const RESOLVED_ID = "\0octarq-plugins";

function installed(name: string): boolean {
  return existsSync(join(WEB_ROOT, "node_modules", ...name.split("/"), "package.json"));
}

function generate(entries: ReturnType<typeof resolveEntries>): string {
  const lines = [
    "// AUTO-GENERATED from the plugin manifest — do not edit.",
    'import { registerUIPlugin } from "@octarq/plugin-sdk";',
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

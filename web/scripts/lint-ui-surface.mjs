import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const webSrc = path.resolve(__dirname, "../src");
const sdkUi = path.resolve(__dirname, "../../packages/plugin-sdk/src/ui");

// Where app-side UI may live. A component in any of these that re-uses an SDK
// export name is the bug this guards: `export *` and an explicit named export
// can coexist, and the explicit one wins — so an app-local Button silently
// replaced the SDK's Button for every "../ui" importer while plugin packages
// (which import @octarq/plugin-sdk directly) kept the SDK's. Two different
// buttons rendered in one product, and nothing failed.
const APP_UI_DIRS = [
  path.join(webSrc, "components"),
  path.join(webSrc, "ui"),
  path.join(webSrc, "app-ui"),
  path.join(webSrc, "dev", "workbench"),
];

function walk(dir) {
  let out = [];
  let entries;
  try {
    entries = fs.readdirSync(dir);
  } catch {
    return out;
  }
  for (const name of entries) {
    const full = path.join(dir, name);
    if (fs.statSync(full).isDirectory()) {
      out = out.concat(walk(full));
    } else if (/\.(ts|tsx)$/.test(name) && !/\.test\./.test(name)) {
      out.push(full);
    }
  }
  return out;
}

// Exported names of a module, from every `export` spelling this repo uses.
// `export * from` contributes nothing statically (it re-exports another
// module's names, which that module's own file already yields).
function exportedNames(src) {
  const names = new Set();
  const decl =
    /^\s*export\s+(?:declare\s+)?(?:async\s+)?(?:function|const|let|var|class|interface|type|enum)\s+([A-Za-z0-9_$]+)/gm;
  for (const m of src.matchAll(decl)) names.add(m[1]);
  const list = /^\s*export\s*(?:type\s*)?\{([^}]*)\}/gm;
  for (const m of src.matchAll(list)) {
    for (const part of m[1].split(",")) {
      const cleaned = part.replace(/^\s*type\s+/, "").trim();
      if (!cleaned) continue;
      const alias = cleaned.split(/\s+as\s+/);
      names.add((alias[1] ?? alias[0]).trim());
    }
  }
  return names;
}

function collect(dir) {
  const map = new Map();
  for (const file of walk(dir)) {
    for (const name of exportedNames(fs.readFileSync(file, "utf8"))) {
      const rel = path.relative(path.resolve(__dirname, ".."), file);
      if (!map.has(name)) map.set(name, []);
      map.get(name).push(rel);
    }
  }
  return map;
}

const sdkNames = collect(sdkUi);
const appNames = new Map();
for (const dir of APP_UI_DIRS) {
  for (const [name, files] of collect(dir)) {
    if (!appNames.has(name)) appNames.set(name, []);
    appNames.get(name).push(...files);
  }
}

let errorCount = 0;
for (const [name, files] of appNames) {
  if (!sdkNames.has(name)) continue;
  for (const file of files) {
    console.error(
      `${file}: re-defines "${name}", which the plugin SDK already exports (packages/plugin-sdk/src/ui) — ` +
        `import it from the SDK instead. An app-local copy shadows the SDK's for every relative importer while ` +
        `plugin packages keep the SDK's, so the product renders two versions of the same component.`,
    );
    errorCount++;
  }
}

if (errorCount > 0) {
  console.error(`\nFound ${errorCount} duplicated UI export name(s) across the app and the SDK.`);
  process.exit(1);
} else {
  console.log("No duplicated UI export names across the app and the SDK.");
  process.exit(0);
}

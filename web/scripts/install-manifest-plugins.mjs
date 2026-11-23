#!/usr/bin/env node
import { existsSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { resolveEntries, parseEntry, WEB_ROOT } from "../plugins-manifest-core.mjs";

const pkg = join(WEB_ROOT, "package.json");
const lock = join(WEB_ROOT, "pnpm-lock.yaml");
const bak = (f) => `${f}.manifest-bak`;

for (const f of [pkg, lock]) {
  const b = bak(f);
  if (existsSync(b)) {
    console.warn(
      `[octarq-plugins] stale ${b} found (previous run was interrupted) — restoring it over ${f}`,
    );
    copyFileSync(b, f);
    rmSync(b);
  }
}

function installed(name) {
  return existsSync(join(WEB_ROOT, "node_modules", ...name.split("/"), "package.json"));
}

const specs = [];
for (const entry of resolveEntries()) {
  const { isLocal, importSpec, installSpec } = parseEntry(entry);
  if (isLocal || !installSpec) continue;
  if (installed(importSpec)) continue;
  specs.push({ name: importSpec, spec: installSpec });
}

if (specs.length === 0) {
  console.log("[octarq-plugins] no external manifest plugins to install");
  process.exit(0);
}

console.log(`[octarq-plugins] installing manifest plugins: ${specs.map((s) => s.spec).join(", ")}`);

const files = [pkg, lock].filter((f) => existsSync(f));

for (const f of files) copyFileSync(f, bak(f));
let failed;
try {
  execFileSync("pnpm", ["add", "-w", ...specs.map((s) => s.spec)], { cwd: WEB_ROOT, stdio: "inherit" });
} catch (err) {
  failed = err;
} finally {
  for (const f of files) {
    if (existsSync(bak(f))) {
      copyFileSync(bak(f), f);
      rmSync(bak(f));
    }
  }
}
if (failed) {
  console.error(`[octarq-plugins] failed to install manifest plugins: ${failed.message}`);
  process.exit(1);
}

const missing = specs.map((s) => s.name).filter((name) => !installed(name));
if (missing.length > 0) {
  console.error(
    `[octarq-plugins] install finished but these packages are missing from node_modules: ${missing.join(", ")}. ` +
      `Check the manifest entries (octarq.plugins.json / OCTARQ_PLUGINS) and the package names.`,
  );
  process.exit(1);
}

console.log(`[octarq-plugins] installed manifest plugins: ${specs.map((s) => s.name).join(", ")}`);

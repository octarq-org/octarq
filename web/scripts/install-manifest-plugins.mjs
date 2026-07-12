#!/usr/bin/env node
// Build-time installer for manifest-defined plugins.
//
// Reads the active plugin manifest (same resolution as the Vite plugin) and
// installs every package specifier it lists that isn't already in node_modules,
// so the manifest-composed build can import third-party plugins by manifest
// alone — the manifest, not this package's package.json, is the source of truth
// for what a build composes.
//
// The install is NOT persisted: package.json and the lockfile are snapshotted
// and restored around `pnpm add`, so the committed files stay clean while the
// fetched plugins remain in node_modules for the subsequent Vite build. Runs
// before `vite build` (see the "build" script).
//
// A `pnpm add` failure is FATAL: an edition build that silently dropped all its
// plugins (bad flag, unreachable registry, typo'd manifest) would ship an empty
// dashboard yet pass CI, which is worse than a loud failure. Editions only list
// plugins they can actually fetch — the OSS default manifest ships the example
// plugin (a workspace member, never installed here), so this step no-ops there.
// Genuine per-entry absence is still tolerated downstream: the Vite plugin skips
// any manifest entry whose package isn't resolvable and its page 404-degrades.
import { existsSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { resolveEntries, parseEntry, WEB_ROOT } from "../plugins-manifest-core.mjs";

function installed(name) {
  return existsSync(join(WEB_ROOT, "node_modules", ...name.split("/"), "package.json"));
}

const specs = [];
for (const entry of resolveEntries()) {
  const { isLocal, importSpec, installSpec } = parseEntry(entry);
  if (isLocal || !installSpec) continue; // local bridges resolve from source, never installed
  if (installed(importSpec)) continue; // already a workspace member / dependency / prior install
  specs.push(installSpec);
}

if (specs.length === 0) {
  console.log("[octarq-plugins] no external manifest plugins to install");
  process.exit(0);
}

console.log(`[octarq-plugins] installing manifest plugins: ${specs.join(", ")}`);

const pkg = join(WEB_ROOT, "package.json");
const lock = join(WEB_ROOT, "pnpm-lock.yaml");
const files = [pkg, lock].filter((f) => existsSync(f));
const bak = (f) => `${f}.manifest-bak`;

for (const f of files) copyFileSync(f, bak(f));
let failed;
try {
  // `pnpm add` (not `install`) — it takes no --frozen-lockfile flag.
  execFileSync("pnpm", ["add", "-O", ...specs], { cwd: WEB_ROOT, stdio: "inherit" });
} catch (err) {
  failed = err;
} finally {
  // Restore the committed files; the fetched packages stay in node_modules.
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

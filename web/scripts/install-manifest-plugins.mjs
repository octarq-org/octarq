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

const pkg = join(WEB_ROOT, "package.json");
const lock = join(WEB_ROOT, "pnpm-lock.yaml");
const bak = (f) => `${f}.manifest-bak`;

// Self-heal from a previous crashed run: if the process was killed mid-install
// (SIGKILL, OOM, power loss) the finally-block restore never ran, leaving a
// stale .manifest-bak next to a dirty live file. Restore the snapshot over the
// live file before doing anything else, so this run starts from the committed
// state instead of compounding on a half-mutated package.json/lockfile.
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
  if (isLocal || !installSpec) continue; // local bridges resolve from source, never installed
  if (installed(importSpec)) continue; // already a workspace member / dependency / prior install
  specs.push(installSpec);
}

if (specs.length === 0) {
  console.log("[octarq-plugins] no external manifest plugins to install");
  process.exit(0);
}

console.log(`[octarq-plugins] installing manifest plugins: ${specs.join(", ")}`);

const files = [pkg, lock].filter((f) => existsSync(f));

for (const f of files) copyFileSync(f, bak(f));
let failed;
try {
  // `pnpm add` (not `install`) — it takes no --frozen-lockfile flag. `-w` targets
  // the workspace root importer (web/ is a workspace root, so a bare add is refused).
  execFileSync("pnpm", ["add", "-O", "-w", ...specs], { cwd: WEB_ROOT, stdio: "inherit" });
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

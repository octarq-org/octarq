/**
 * Post-build guard for the /api-reference/ page's interactive explorer.
 *
 * Reads the built HTML (and the scripts it references) from dist/ — not the
 * source — so a broken render chain (mount never emitted, Scalar bundle
 * dropped, or a CDN script sneaking back in) fails here instead of shipping.
 *
 * Run via `pnpm check:api-reference` after `pnpm build`.
 */

import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const pageDir = join(root, "dist", "api-reference");

const failures = [];
const report = (label, message) => failures.push(`${label}: ${message}`);

const htmlPath = ["index.html", "index.htm"]
  .map((file) => join(pageDir, file))
  .find((path) => existsSync(path));

if (!htmlPath) {
  report("page missing", `no built HTML found under ${pageDir}`);
} else {
  const html = readFileSync(htmlPath, "utf8");

  // The mount point the client script renders Scalar into. If the render
  // chain breaks (route stops emitting the component, file deleted, …), this
  // disappears from the build output and the check goes red.
  if (!html.includes("data-api-reference-explorer")) {
    report("mount point", 'Scalar mount element ("data-api-reference-explorer") is missing from the built page');
  }

  // The wrapper above is only the styled box. The client script mounts into
  // `#api-reference` (getElementById), so losing *that* id renders nothing at
  // all while the wrapper still ships — an empty bordered rectangle, which is
  // precisely the failure this page was in before. Check the id itself.
  if (!/id="api-reference"/.test(html)) {
    report("mount target", 'the "#api-reference" element the client script mounts into is missing from the built page');
  }

  // Guard against regressing to the old RapiDoc markup.
  if (html.includes("rapi-doc")) {
    report("legacy explorer", 'built page still contains the legacy <rapi-doc> markup');
  }

  // No third-party script origins. The page may only load its own /_astro/ assets.
  const externalSrcs = [...html.matchAll(/<script[^>]*\bsrc="(https?:\/\/[^"]+)"/gi)].map((m) => m[1]);
  if (externalSrcs.length > 0) {
    report("external scripts", `page loads scripts from third-party hosts: ${externalSrcs.join(", ")}`);
  }
  for (const host of ["unpkg.com", "cdn.jsdelivr.net"]) {
    if (html.includes(host)) {
      report("cdn host", `built page references "${host}"`);
    }
  }

  // The Scalar bundle must actually be wired into the page. Astro emits the
  // component's script as a hashed /_astro/*.js asset referenced by the HTML;
  // confirm at least one of those assets contains the Scalar reference code.
  const assetSrcs = [...html.matchAll(/src="(\/_astro\/[^"]+\.js)"/g)].map((m) => m[1]);
  const scalarBundled = assetSrcs.some((src) => {
    const assetPath = join(root, "dist", src.replace(/^\//, ""));
    return existsSync(assetPath) && readFileSync(assetPath, "utf8").includes("scalar-api-reference");
  });
  if (!scalarBundled) {
    report("scalar bundle", "none of the page's /_astro/*.js assets contain the Scalar reference code");
  }
}

if (failures.length > 0) {
  console.error("check-api-reference FAILED");
  for (const failure of failures) {
    console.error(`  \u2717 ${failure}`);
  }
  process.exit(1);
}

console.log("check-api-reference OK: Scalar explorer present, no third-party scripts");

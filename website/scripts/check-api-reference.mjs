
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

  if (!html.includes("data-api-reference-explorer")) {
    report("mount point", 'Scalar mount element ("data-api-reference-explorer") is missing from the built page');
  }

  if (!/id="api-reference"/.test(html)) {
    report("mount target", 'the "#api-reference" element the client script mounts into is missing from the built page');
  }

  if (html.includes("rapi-doc")) {
    report("legacy explorer", 'built page still contains the legacy <rapi-doc> markup');
  }

  const FIRST_PARTY_SCRIPT_HOSTS = ["plausible.jungley.net"];
  const externalSrcs = [...html.matchAll(/<script[^>]*\bsrc="(https?:\/\/[^"]+)"/gi)]
    .map((m) => m[1])
    .filter((src) => {
      try {
        return !FIRST_PARTY_SCRIPT_HOSTS.includes(new URL(src).host);
      } catch {
        return true;
      }
    });
  if (externalSrcs.length > 0) {
    report("external scripts", `page loads scripts from third-party hosts: ${externalSrcs.join(", ")}`);
  }
  for (const host of ["unpkg.com", "cdn.jsdelivr.net"]) {
    if (html.includes(host)) {
      report("cdn host", `built page references "${host}"`);
    }
  }

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

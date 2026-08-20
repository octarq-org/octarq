import { execSync } from "node:child_process";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const root = dirname(dirname(fileURLToPath(import.meta.url)));

try {
  console.log("[website] Generating openapi.json from Go handlers...");
  execSync("go run ../cmd/openapi-gen > public/openapi.json", {
    cwd: root,
    stdio: ["ignore", "pipe", "inherit"],
  });
  console.log("[website] Successfully generated public/openapi.json");
} catch (e) {
  if (existsSync(join(root, "public", "openapi.json"))) {
    console.warn("[website] Warning: go toolchain unavailable or error generating spec, using existing public/openapi.json fallback");
  } else {
    console.error("[website] Error: failed to generate openapi.json and no fallback exists");
    throw e;
  }
}

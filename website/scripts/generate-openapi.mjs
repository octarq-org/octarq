import { execSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname } from "node:path";

const root = dirname(dirname(fileURLToPath(import.meta.url)));

console.log("[website] Generating public/openapi.json from Go handlers...");
try {
  execSync("go run ../cmd/openapi-gen > public/openapi.json", {
    cwd: root,
    stdio: ["ignore", "pipe", "inherit"],
  });
  console.log("[website] Successfully generated public/openapi.json");
} catch (err) {
  console.error(
    "[website] ERROR: Failed to generate public/openapi.json from Go handlers.\n" +
    "[website] Go toolchain is required to build the website documentation. Aborting."
  );
  process.exit(1);
}

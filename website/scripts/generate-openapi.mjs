import { execSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const serverDir = join(root, "..", "server");

console.log("[website] Generating public/openapi.json from Go handlers...");
try {
  execSync("go run cmd/openapi-gen/main.go > " + join(root, "public", "openapi.json"), {
    cwd: serverDir,
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

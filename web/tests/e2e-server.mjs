import { execFileSync, spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const webDir = path.join(repoRoot, "web");
const serverDir = path.join(repoRoot, "server");
const binary = path.join(repoRoot, "octarq");

console.log("[e2e] building the dashboard (server/webembed/dist)…");
execFileSync("pnpm", ["build"], { cwd: webDir, stdio: "inherit" });

console.log("[e2e] building the octarq binary…");
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], {
  cwd: serverDir,
  stdio: "inherit",
  env: { ...process.env, CGO_ENABLED: "0" },
});

const server = spawn(binary, [], { env: process.env, stdio: "inherit" });

for (const signal of ["SIGTERM", "SIGINT", "SIGHUP"]) {
  process.on(signal, () => server.kill(signal));
}
server.on("exit", (code, signal) => {
  process.exit(code ?? (signal ? 1 : 0));
});

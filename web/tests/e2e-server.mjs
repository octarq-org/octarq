// Playwright webServer bootstrap for the octarq e2e suite.
//
// `pnpm e2e` must work from a clean checkout with zero manual setup, so this
// command builds whatever a previous step hasn't, then boots the backend with
// the environment playwright.config.ts set up and stays attached until
// Playwright kills us.
//
// Why build anything at all here: the single binary embeds webembed/dist at
// compile time, and the committed dist is stale on any branch that touches
// web/ (see CLAUDE.md — keeping it current is CI's job, not yours). Serving the
// stale dist would make the suite test a dashboard nobody is looking at, so the
// dashboard is always rebuilt before the binary. Both are incremental (Vite /
// Go caches), so unchanged reruns stay fast.
import { execFileSync, spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const webDir = path.join(repoRoot, "web");
const binary = path.join(repoRoot, "octarq");

console.log("[e2e] building the dashboard (webembed/dist)…");
execFileSync("pnpm", ["build"], { cwd: webDir, stdio: "inherit" });

// -buildvcs=false: this is a throwaway test server, so the VCS stamp buys
// nothing, and obtaining it means shelling out to git. Under a container whose
// uid does not own the checkout — which is how the e2e job runs — git refuses
// with "dubious ownership" and the build fails as
// `error obtaining VCS status: exit status 128`, before a single test runs.
console.log("[e2e] building the octarq binary…");
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], {
  cwd: repoRoot,
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

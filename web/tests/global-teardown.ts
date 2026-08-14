import { rmSync } from "node:fs";

// Removes the per-run temp dir (fresh sqlite DB + any auto-generated state)
// that playwright.config.ts created. Runs after reporters, so the HTML report
// is already on disk and unaffected.
export default function globalTeardown() {
  const dir = process.env.OCTARQ_E2E_TMPDIR;
  if (dir) {
    rmSync(dir, { recursive: true, force: true });
  }
}

import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { defineConfig, devices } from "@playwright/test";

// ─── e2e environment (single source of truth) ────────────────────────────────
// These defaults are injected into process.env before the webServer child and
// the specs read them, so the admin credentials live in exactly one place.
// Anything already exported by the shell wins (`??=`), which is what a mutation
// check uses to feed the suite a deliberately-wrong password.
process.env.OCTARQ_E2E_PORT ||= "8377";
process.env.OCTARQ_ADMIN_USER ||= "e2e-admin@example.com";
process.env.OCTARQ_ADMIN_PASSWORD ||= "e2e-admin-password";

// A throwaway sqlite file in a fresh temp dir per run — never the repo-root
// octarq.db. tests/global-teardown.ts deletes the dir once the run is over.
process.env.OCTARQ_E2E_TMPDIR = mkdtempSync(path.join(tmpdir(), "octarq-e2e-"));

const e2ePort = Number(process.env.OCTARQ_E2E_PORT);
const baseURL = `http://127.0.0.1:${e2ePort}`;

// Backend env for the webServer child. Forced here (not `??=`) so the port
// always matches baseURL and the DB can never be anything but a fresh temp file.
process.env.OCTARQ_LISTEN = `:${e2ePort}`;
process.env.OCTARQ_DB_DRIVER = "sqlite";
process.env.OCTARQ_DB_DSN = path.join(process.env.OCTARQ_E2E_TMPDIR, "octarq-e2e.db");
process.env.OCTARQ_SECRET_KEY = "octarq-e2e-secret-key-not-for-real-use";

export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  retries: 0,
  workers: 1,
  reporter: "html",
  globalTeardown: "./tests/global-teardown.ts",
  use: {
    baseURL,
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: {
    // Builds whatever a clean checkout lacks (dashboard + Go binary) and boots
    // the backend with the env above; see tests/e2e-server.mjs.
    command: "node tests/e2e-server.mjs",
    // /api/health returns 503 until the database answers, so Playwright only
    // considers the server ready once the app is actually usable.
    url: `${baseURL}/api/health`,
    timeout: 240_000,
    // Local reruns reuse a still-running server for speed; CI always starts
    // fresh so every run exercises the full bootstrap.
    reuseExistingServer: !process.env.CI,
    env: { ...process.env },
  },
});

import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { defineConfig, devices } from "@playwright/test";

process.env.OCTARQ_E2E_PORT ||= "8377";
process.env.OCTARQ_ADMIN_USER ||= "e2e-admin@example.com";
process.env.OCTARQ_ADMIN_PASSWORD ||= "e2e-admin-password";

process.env.OCTARQ_E2E_TMPDIR = mkdtempSync(path.join(tmpdir(), "octarq-e2e-"));

const e2ePort = Number(process.env.OCTARQ_E2E_PORT);
const baseURL = `http://127.0.0.1:${e2ePort}`;

process.env.OCTARQ_LISTEN = `:${e2ePort}`;
process.env.OCTARQ_DB_DRIVER = "sqlite";
process.env.OCTARQ_DB_DSN = path.join(process.env.OCTARQ_E2E_TMPDIR, "octarq-e2e.db");
process.env.OCTARQ_SECRET_KEY = "octarq-e2e-secret-key-not-for-real-use";
process.env.OCTARQ_RATELIMIT_AUTH_RPM = "3000";
process.env.OCTARQ_RATELIMIT_API_RPM = "3000";


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
    command: "node tests/e2e-server.mjs",
    url: `${baseURL}/api/health`,
    timeout: 240_000,
    reuseExistingServer: !process.env.CI,
    env: { ...process.env },
  },
});

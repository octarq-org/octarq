import { test, expect } from "@playwright/test";

// Fixed mock payloads matching the shape from internal/api/health.go
// (StatusStatusOutputBody) and web/src/api.ts (SubsystemStatusResponse).
const subsystemsOk = [
  { name: "database", status: "ok" },
  { name: "mail", status: "ok" },
  { name: "queue", status: "ok" },
];

const subsystemsDegraded = [
  { name: "database", status: "ok" },
  { name: "mail", status: "degraded", detail: "SMTP timeout" },
  { name: "queue", status: "ok" },
];

const subsystemsDown = [
  { name: "database", status: "down", detail: "connection refused" },
  { name: "mail", status: "ok" },
  { name: "queue", status: "ok" },
];

function mockStatus(
  overall: "ok" | "degraded" | "down",
  subsystems: typeof subsystemsOk,
) {
  return {
    overall,
    subsystems,
    time: new Date().toISOString(),
  };
}

test.describe("status page", () => {
  // The status page is public — no login required.

  test("shows All Systems Operational when overall is ok", async ({ page }) => {
    await page.route("**/api/status", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(mockStatus("ok", subsystemsOk)),
      }),
    );
    await page.goto("/status");
    await expect(page.getByText("All Systems Operational")).toBeVisible({
      timeout: 15000,
    });
    await page.screenshot({
      path: "test-results/status-ok.png",
      fullPage: true,
    });
  });

  test("shows Some Systems Degraded when overall is degraded", async ({
    page,
  }) => {
    await page.route("**/api/status", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(mockStatus("degraded", subsystemsDegraded)),
      }),
    );
    await page.goto("/status");
    await expect(page.getByText("Some Systems Degraded")).toBeVisible({
      timeout: 15000,
    });
    await page.screenshot({
      path: "test-results/status-degraded.png",
      fullPage: true,
    });
  });

  test("shows Major Service Outage when overall is down", async ({ page }) => {
    await page.route("**/api/status", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(mockStatus("down", subsystemsDown)),
      }),
    );
    await page.goto("/status");
    await expect(page.getByText("Major Service Outage")).toBeVisible({
      timeout: 15000,
    });
    await page.screenshot({
      path: "test-results/status-down.png",
      fullPage: true,
    });
  });
});

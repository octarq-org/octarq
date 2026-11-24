import { mkdirSync } from "node:fs";
import path from "node:path";
import { test, expect } from "@playwright/test";
import { ADMIN_USER, ADMIN_PASSWORD, signIn, attachPageGuards, expectNoI18nLeak } from "./helpers";

const ARTIFACT_DIR = path.resolve(process.cwd(), "test-results", "help-drawer");

test.describe("Ambient help drawer", () => {
  test("opens the doc mapped to the current page and closes with Esc", async ({ page }) => {
    mkdirSync(ARTIFACT_DIR, { recursive: true });
    const guard = attachPageGuards(page);

    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.goto("/admin/settings/webhooks");
    await page.waitForLoadState("networkidle");
    expectNoI18nLeak(page);

    await page.getByRole("button", { name: "Open help" }).click();
    const drawer = page.getByTestId("help-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByRole("heading", { name: "Verify Webhook Signatures" })).toBeVisible();

    for (const theme of ["light", "dark"] as const) {
      for (const viewport of [
        { name: "desktop", width: 1440, height: 900 },
        { name: "mobile", width: 390, height: 844 },
      ] as const) {
        await page.setViewportSize({ width: viewport.width, height: viewport.height });
        await page.evaluate((target) => {
          localStorage.setItem("octarq-theme", target);
          document.documentElement.classList.toggle("dark", target === "dark");
        }, theme);
        await page.screenshot({
          path: path.join(ARTIFACT_DIR, `drawer-${theme}-${viewport.name}.png`),
          fullPage: false,
        });
      }
    }

    await page.setViewportSize({ width: 1440, height: 900 });
    await page.keyboard.press("Escape");
    await expect(drawer).toBeHidden();

    await page.keyboard.press("F1");
    await expect(drawer).toBeVisible();
    const url = new URL(page.url());
    expect(url.searchParams.get("help")).toBe("webhooks");

    guard.assertClean();
  });

  test("deep links open the drawer directly and survive reload", async ({ page }) => {
    const guard = attachPageGuards(page);
    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.goto("/admin/settings/webhooks?help=webhooks");
    await page.waitForLoadState("networkidle");

    const drawer = page.getByTestId("help-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByRole("heading", { name: "Verify Webhook Signatures" })).toBeVisible();

    await page.reload();
    await page.waitForLoadState("networkidle");
    await expect(page.getByTestId("help-drawer")).toBeVisible();

    guard.assertClean();
  });
});

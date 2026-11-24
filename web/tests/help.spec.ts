import { mkdirSync } from "node:fs";
import path from "node:path";
import { test, expect } from "@playwright/test";
import { ADMIN_USER, ADMIN_PASSWORD, signIn, attachPageGuards, expectNoCommentLeak, expectNoI18nLeak } from "./helpers";

const ARTIFACT_DIR = path.resolve(process.cwd(), "test-results", "help-center");

test.describe("Help center", () => {
  test("hub lists categories and opens a doc with a readable article", async ({ page }) => {
    mkdirSync(ARTIFACT_DIR, { recursive: true });
    const guard = attachPageGuards(page);

    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.goto("/admin/help");
    await page.waitForLoadState("networkidle");
    expectNoCommentLeak(page);
    expectNoI18nLeak(page);

    await expect(page.getByRole("heading", { name: "Documentation", exact: true })).toBeVisible();
    await expect(page.getByPlaceholder("Search the documentation")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Quickstart" })).toBeVisible();

    const firstDoc = page.locator('[data-testid^="help-doc-"]').first();
    await expect(firstDoc).toBeVisible();
    await firstDoc.click();

    await page.waitForLoadState("networkidle");
    await expect(page).toHaveURL(/\/admin\/help\/[a-z]+\/[a-z0-9-]+$/);
    await expect(page.getByRole("heading", { level: 1 })).not.toBeEmpty();

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
        await page.goto("/admin/help");
        await page.waitForLoadState("networkidle");
        await expect(page.getByRole("heading", { name: "Documentation", exact: true })).toBeVisible();
        await page.screenshot({
          path: path.join(ARTIFACT_DIR, `hub-${theme}-${viewport.name}.png`),
          fullPage: true,
        });
        await expect(page.locator("#root")).not.toBeEmpty();
      }
    }

    guard.assertClean();
  });

  test("mobile readers reach the table of contents from the sticky pill", async ({ page }) => {
    const guard = attachPageGuards(page);
    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/admin/help/start/quickstart");
    await page.waitForLoadState("networkidle");
    expectNoI18nLeak(page);

    const pill = page.getByRole("button", { name: "On this page" });
    await expect(pill).toBeVisible();
    await pill.click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toBeHidden();

    guard.assertClean();
  });
});

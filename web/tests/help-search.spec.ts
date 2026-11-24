import { mkdirSync } from "node:fs";
import path from "node:path";
import { test, expect } from "@playwright/test";
import { ADMIN_USER, ADMIN_PASSWORD, signIn, attachPageGuards, expectNoI18nLeak } from "./helpers";

const ARTIFACT_DIR = path.resolve(process.cwd(), "test-results", "help-search");

test.describe("Help search", () => {
  test("searches doc bodies and navigates to the matched anchor", async ({ page }) => {
    mkdirSync(ARTIFACT_DIR, { recursive: true });
    const guard = attachPageGuards(page);

    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.goto("/admin/help");
    await page.waitForLoadState("networkidle");
    expectNoI18nLeak(page);

    const search = page.getByRole("combobox", { name: "Search documentation" });
    await search.fill("webhook");

    const listbox = page.getByRole("listbox", { name: "Search results" });
    await expect(listbox).toBeVisible();
    const options = listbox.getByRole("option");
    await expect(options.first()).toBeVisible();
    await expect(listbox).toContainText("Webhook");

    await page.screenshot({
      path: path.join(ARTIFACT_DIR, "search-webhook-desktop.png"),
      fullPage: false,
    });

    await search.press("Enter");
    await page.waitForLoadState("networkidle");
    await expect(page).toHaveURL(/\/admin\/help\/[a-z]+\/[a-z0-9-]+/);
    await expect(page.getByRole("heading", { level: 1 })).not.toBeEmpty();

    guard.assertClean();
  });

  test("empty and unmatched queries show explicit states", async ({ page }) => {
    const guard = attachPageGuards(page);
    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.goto("/admin/help");
    await page.waitForLoadState("networkidle");

    const search = page.getByRole("combobox", { name: "Search documentation" });
    await search.fill("zzzzqxjw");
    await expect(page.getByText(/No results for/)).toBeVisible();

    await search.press("Escape");
    await expect(page.getByRole("listbox")).toBeHidden();

    guard.assertClean();
  });
});

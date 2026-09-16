import { test, expect } from "@playwright/test";
import {
  ADMIN_USER,
  ADMIN_PASSWORD,
  signIn,
  expectNoCommentLeak,
  expectNoI18nLeak,
  attachPageGuards,
} from "./helpers";

test.describe("Links Management E2E Journey", () => {
  test("creates a new shortlink and displays it in the links catalog", async ({ page }) => {
    const guard = attachPageGuards(page);

    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await expect(page.getByRole("button", { name: "Settings" })).toBeVisible();

    // Navigate to links management
    await page.goto("/admin/links");
    await page.waitForLoadState("networkidle");

    // Open the creation modal
    const createBtn = page.getByRole("button", { name: /New Link/i }).first();
    await expect(createBtn).toBeVisible();
    await createBtn.click();

    // Fill link form
    const targetInput = page.getByPlaceholder(/https:\/\//i).first();
    await expect(targetInput).toBeVisible();
    await targetInput.fill("https://example.org/playwright-landing");

    const slugInput = page.getByPlaceholder(/e\.g\. promo2026/i);
    await expect(slugInput).toBeVisible();
    const testSlug = `e2e-${Date.now().toString(36)}`;
    await slugInput.fill(testSlug);

    // Save link
    const saveBtn = page.getByRole("button", { name: /Save Link/i });
    await saveBtn.click();

    // Verify it appears in the link list
    await expect(page.getByText(testSlug).first()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("https://example.org/playwright-landing").first()).toBeVisible();

    // Verify search filter works
    const searchInput = page.getByPlaceholder(/Search links/i);
    await searchInput.fill(testSlug);
    await expect(page.getByText(testSlug).first()).toBeVisible();

    // Verify backstop guards
    await expectNoCommentLeak(page);
    await expectNoI18nLeak(page);
    guard.assertClean();
  });
});

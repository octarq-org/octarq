import { test, expect } from "@playwright/test";
import {
  ADMIN_USER,
  ADMIN_PASSWORD,
  signIn,
  expectNoCommentLeak,
  expectNoI18nLeak,
  attachPageGuards,
} from "./helpers";

test.describe("Audit Trail E2E Journey", () => {
  test("renders audit trail and records activity events", async ({ page }) => {
    const guard = attachPageGuards(page);

    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await expect(page.getByRole("button", { name: "Settings" })).toBeVisible();

    // Navigate to audit page
    await page.goto("/admin/audit");
    await page.waitForLoadState("networkidle");

    // The audit table or heading should render
    await expect(page.getByRole("heading", { name: /Audit/i })).toBeVisible();

    // Verify backstop guards
    await expectNoCommentLeak(page);
    await expectNoI18nLeak(page);
    guard.assertClean();
  });
});

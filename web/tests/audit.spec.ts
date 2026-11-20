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

    await page.goto("/admin/governance/audit");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("heading", { name: /Audit/i })).toBeVisible();

    await expectNoCommentLeak(page);
    await expectNoI18nLeak(page);
    guard.assertClean();
  });
});

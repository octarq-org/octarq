import { test, expect } from "@playwright/test";
import {
  ADMIN_USER,
  ADMIN_PASSWORD,
  signIn,
  expectNoCommentLeak,
  expectNoI18nLeak,
  attachPageGuards,
} from "./helpers";

test.describe("Team & Members Management E2E Journey", () => {
  test("displays current workspace members and invite controls for admin", async ({ page }) => {
    const guard = attachPageGuards(page);

    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await expect(page.getByRole("button", { name: "Settings" })).toBeVisible();

    // Navigate to members settings page
    await page.goto("/admin/settings/members");
    await page.waitForLoadState("networkidle");

    // The current admin user's email should be listed in the members roster
    await expect(page.getByText(ADMIN_USER).first()).toBeVisible();

    // Admin should see role badge
    await expect(page.getByText(/owner|admin/i).first()).toBeVisible();

    // Verify invite / add member form is accessible to admin
    await expect(page.getByPlaceholder("colleague@example.com")).toBeVisible();
    await expect(page.getByRole("button", { name: /Invite Member/i })).toBeVisible();

    // Verify backstop guards
    await expectNoCommentLeak(page);
    await expectNoI18nLeak(page);
    guard.assertClean();
  });
});

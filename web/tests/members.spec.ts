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

    await page.goto("/admin/settings/members");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(ADMIN_USER).first()).toBeVisible();

    await expect(page.getByText(/owner|admin/i).first()).toBeVisible();

    await expect(page.getByPlaceholder("colleague@example.com")).toBeVisible();
    await expect(page.getByRole("button", { name: /Invite Member/i })).toBeVisible();

    await expectNoCommentLeak(page);
    await expectNoI18nLeak(page);
    guard.assertClean();
  });
});

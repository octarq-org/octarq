import { test, expect } from "@playwright/test";
import { ADMIN_USER, ADMIN_PASSWORD, WRONG_PASSWORD, signIn, expectNoCommentLeak } from "./helpers";

test.describe("auth", () => {
  test("signs in with the admin account and lands in the backoffice", async ({ page }) => {
    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);

    // The shell's settings gear only renders once authenticated.
    await expect(page.getByRole("button", { name: "Settings" })).toBeVisible();
    await expect(page).toHaveURL(/\/admin\/overview/);

    // Regression guard for the /* ui-color-ok */ incident: no comment marker
    // may render as visible text in the backoffice.
    await expectNoCommentLeak(page);
  });

  test("rejects a wrong password without entering the backoffice", async ({ page }) => {
    await signIn(page, ADMIN_USER, WRONG_PASSWORD);

    await expect(page.getByText("invalid credentials")).toBeVisible();
    // Still on the login screen — the backoffice chrome never appears.
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Settings" })).toHaveCount(0);
  });

  test("keeps an unauthenticated visitor off a protected route", async ({ page }) => {
    await page.goto("/admin/settings/general");

    // The login screen is shown instead of the settings page.
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "General" })).toHaveCount(0);
  });
});

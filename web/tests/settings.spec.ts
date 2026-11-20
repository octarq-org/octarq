import { test, expect } from "@playwright/test";
import { ADMIN_USER, ADMIN_PASSWORD, signIn, expectNoCommentLeak } from "./helpers";

test.describe("Settings", () => {
  test("persists GitHub OAuth credentials on the Authentication page", async ({ page }) => {
    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await expect(page.getByRole("button", { name: "Settings" })).toBeVisible();
    await page.goto("/admin/settings/instance/auth");

    await page.getByRole("button", { name: /GitHub/ }).click();

    const clientId = page.getByPlaceholder("Ov23li…");
    await expect(clientId).toBeVisible();
    await clientId.fill("e2e-github-client-id");
    await page.getByPlaceholder("Secret value").fill("e2e-github-client-secret");
    await page.getByRole("button", { name: "Save" }).click();

    await expect(page.getByText("✓ Saved")).toBeVisible();

    await page.reload();
    await page.getByRole("button", { name: /GitHub/ }).click();
    await expect(page.getByPlaceholder("Ov23li…")).toHaveValue("e2e-github-client-id");

    await expectNoCommentLeak(page);
  });
});

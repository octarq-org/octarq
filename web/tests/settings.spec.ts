import { test, expect } from "@playwright/test";
import { ADMIN_USER, ADMIN_PASSWORD, signIn, expectNoCommentLeak } from "./helpers";

// Replaces the original spec, which was written against UI that no longer
// exists and never ran:
//  - "Update data retention setting" targeted a retention number input on
//    /admin/settings/general that the page has never had (only a workspace
//    rename form lives there) — deleted.
//  - "OAuth settings configuration" targeted /admin/settings/general but the
//    GitHub/Google provider config moved to /admin/settings/instance/auth (an
//    accordion list) — rewritten against the real page below.
//  - "/admin/license redirects" asserted a redirect to a settings page that the
//    OSS build does not ship (/settings/license renders nothing), so its
//    assertions were trivially true — deleted.
test.describe("Settings", () => {
  test("persists GitHub OAuth credentials on the Authentication page", async ({ page }) => {
    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    // Wait for the authenticated shell before navigating: a goto issued while
    // the login POST is still in flight aborts the response and the session
    // cookie never reaches the browser, stranding the test on the login page.
    await expect(page.getByRole("button", { name: "Settings" })).toBeVisible();
    await page.goto("/admin/settings/instance/auth");

    // Expand the GitHub provider row (accordion) to reveal its config.
    await page.getByRole("button", { name: /GitHub/ }).click();

    const clientId = page.getByPlaceholder("Ov23li…");
    await expect(clientId).toBeVisible();
    await clientId.fill("e2e-github-client-id");
    await page.getByPlaceholder("Secret value").fill("e2e-github-client-secret");
    await page.getByRole("button", { name: "Save" }).click();

    await expect(page.getByText("✓ Saved")).toBeVisible();

    // Reload: the client id persists (the secret is stored encrypted).
    await page.reload();
    await page.getByRole("button", { name: /GitHub/ }).click();
    await expect(page.getByPlaceholder("Ov23li…")).toHaveValue("e2e-github-client-id");

    // The settings page is a backoffice page, so the comment-leak backstop
    // applies here too.
    await expectNoCommentLeak(page);
  });
});

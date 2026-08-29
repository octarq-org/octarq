import { expect, type Page } from "@playwright/test";

// The credentials come from the same process.env that playwright.config.ts
// hands the webServer — never a second literal in the specs.
export function e2eEnv(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(`e2e: ${name} is not set — run via \`pnpm e2e\` (playwright.config.ts sets it)`);
  }
  return value;
}

export const ADMIN_USER = e2eEnv("OCTARQ_ADMIN_USER");
export const ADMIN_PASSWORD = e2eEnv("OCTARQ_ADMIN_PASSWORD");

// Deliberately not the admin password — the whole point is that it is wrong.
export const WRONG_PASSWORD = "e2e-wrong-password-not-the-real-one";

export async function signIn(page: Page, email: string, password: string) {
  await page.goto("/admin");
  await page.fill("#login-email", email);
  await page.fill("#login-password", password);
  await page.getByRole("button", { name: "Sign In" }).click();
  await dismissOnboardingIfNeeded(page);
}

async function dismissOnboardingIfNeeded(page: Page) {
  const startSetup = page.getByRole("button", { name: /Start Setup|开始搭建/i });
  const skip = page.getByRole("button", { name: /^Skip$|^跳过$/i });
  const settings = page.getByRole("button", { name: "Settings" });
  let dismissedViaUI = false;
  try {
    await Promise.race([
      startSetup.waitFor({ state: "visible", timeout: 6000 }),
      settings.waitFor({ state: "visible", timeout: 6000 }),
    ]);
    if (await startSetup.isVisible().catch(() => false)) {
      await startSetup.click();
      await skip.waitFor({ state: "visible", timeout: 4000 });
      await skip.click();
      dismissedViaUI = true;
    } else {
      return;
    }
  } catch {}
  if (!dismissedViaUI) return;
  try {
    await page.evaluate(() => {
      try {
        localStorage.setItem("onboarding_completed", "true");
        localStorage.setItem("octarq:onboarding:completed", "true");
      } catch {}
    });
    await page.evaluate(() =>
      fetch("/api/user/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ key: "onboarding_completed", value: "true" }),
      }).catch(() => {}),
    );
    await page.waitForTimeout(400);
    const hasSettings = await settings.isVisible().catch(() => false);
    if (!hasSettings) {
      await page.goto("/admin/overview");
    }
    await settings.waitFor({ state: "visible", timeout: 8000 }).catch(() => {});
  } catch {}
}

// End-to-end backstop for the `/* ui-color-ok */` incident: a JS comment that
// lands in JSX children position (after `/>` or after an opening tag's `>`)
// renders as literal visible text. The page's visible text must never contain
// a comment marker. (The component-level guard for this lives elsewhere; this
// is the runtime layer that catches a leak that made it to a real page.)
export async function expectNoCommentLeak(page: Page) {
  // The backoffice mounts route content asynchronously (lazy chunks + data
  // fetches), so read the text only once the network has settled — otherwise
  // the check runs while the page is still the shell and misses page leaks.
  await page.waitForLoadState("networkidle");
  const text = await page.locator("body").innerText();
  expect(text).not.toMatch(/\/\*|\*\//);
}

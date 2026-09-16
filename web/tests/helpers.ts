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

// Runtime backstop for unhandled errors and server 500 responses.
export interface PageGuard {
  pageErrors: Error[];
  consoleErrors: string[];
  serverErrors: string[];
  assertClean(): void;
}

export function attachPageGuards(page: Page): PageGuard {
  const pageErrors: Error[] = [];
  const consoleErrors: string[] = [];
  const serverErrors: string[] = [];

  page.on("pageerror", (err) => {
    pageErrors.push(err);
  });

  page.on("console", (msg) => {
    if (msg.type() === "error") {
      const text = msg.text();
      // Ignore browser network status messages and favicon 404s
      if (!text.includes("favicon.ico") && !text.includes("Failed to load resource")) {
        consoleErrors.push(text);
      }
    }
  });

  page.on("response", (res) => {
    if (res.status() >= 500) {
      serverErrors.push(`${res.status()} ${res.url()}`);
    }
  });

  return {
    pageErrors,
    consoleErrors,
    serverErrors,
    assertClean() {
      expect(pageErrors, `Uncaught page errors: ${pageErrors.map(e => e.message).join(", ")}`).toHaveLength(0);
      expect(consoleErrors, `Leaked console.error: ${consoleErrors.join("; ")}`).toHaveLength(0);
      expect(serverErrors, `Server 5xx errors: ${serverErrors.join("; ")}`).toHaveLength(0);
    },
  };
}

// Runtime backstop for missing translation keys.
export async function expectNoI18nLeak(page: Page) {
  await page.waitForLoadState("networkidle");
  const text = await page.locator("body").innerText();
  expect(text).not.toMatch(/\[missing\s+"[^"]+"\]/i);
}


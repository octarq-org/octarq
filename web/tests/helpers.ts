import { expect, type Page } from "@playwright/test";

export function e2eEnv(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(`e2e: ${name} is not set — run via \`pnpm e2e\` (playwright.config.ts sets it)`);
  }
  return value;
}

export const ADMIN_USER = e2eEnv("OCTARQ_ADMIN_USER");
export const ADMIN_PASSWORD = e2eEnv("OCTARQ_ADMIN_PASSWORD");

export const WRONG_PASSWORD = "e2e-wrong-password-not-the-real-one";

export async function signIn(page: Page, email: string, password: string) {
  await page.goto("/admin");
  await page.fill("#login-email", email);
  await page.fill("#login-password", password);
  await page.getByRole("button", { name: "Sign In" }).click();
}

// End-to-end backstop for the `/* ui-color-ok */` incident: a JS comment that
export async function expectNoCommentLeak(page: Page) {
  await page.waitForLoadState("networkidle");
  const text = await page.locator("body").innerText();
  expect(text).not.toMatch(/\/\*|\*\//);
}

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

export async function expectNoI18nLeak(page: Page) {
  await page.waitForLoadState("networkidle");
  const text = await page.locator("body").innerText();
  expect(text).not.toMatch(/\[missing\s+"[^"]+"\]/i);
}


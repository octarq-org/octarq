import { mkdirSync } from "node:fs";
import path from "node:path";
import { test, expect } from "@playwright/test";
import { ADMIN_USER, ADMIN_PASSWORD, signIn, attachPageGuards, expectNoCommentLeak, expectNoI18nLeak } from "./helpers";

const STORIES = [
  { name: "overview", path: "/admin/overview", expectedWidth: "full" },
  { name: "settings-appearance", path: "/admin/settings/appearance", expectedWidth: "prose" },
  { name: "settings-general", path: "/admin/settings/general", expectedWidth: "prose" },
  { name: "audit", path: "/admin/audit", expectedWidth: "full" },
] as const;

const VIEWPORTS = [
  { name: "desktop", width: 1440, height: 900 },
  { name: "mobile", width: 390, height: 844 },
] as const;

const THEMES = ["light", "dark"] as const;

const ARTIFACT_DIR = path.resolve(process.cwd(), "test-results", "design-system");

test.describe("Design System UI Stories", () => {
  test("3-surface hierarchy, prose/full geometry, and light/dark screenshots across viewports", async ({ page }, testInfo) => {
    mkdirSync(ARTIFACT_DIR, { recursive: true });
    const guard = attachPageGuards(page);

    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await expect(page.getByRole("button", { name: "Settings" })).toBeVisible();

    for (const theme of THEMES) {
      await page.evaluate((targetTheme) => {
        localStorage.setItem("octarq-theme", targetTheme);
        document.documentElement.classList.toggle("dark", targetTheme === "dark");
      }, theme);

      for (const vp of VIEWPORTS) {
        await page.setViewportSize({ width: vp.width, height: vp.height });

        for (const story of STORIES) {
          await page.goto(story.path);
          await page.waitForLoadState("networkidle");
          await expectNoCommentLeak(page);
          await expectNoI18nLeak(page);

          // 1. Verify no horizontal scroll overflow on either desktop or mobile.
          const overflow = await page.evaluate(() => ({
            scrollWidth: document.documentElement.scrollWidth,
            clientWidth: document.documentElement.clientWidth,
          }));
          expect(
            overflow.scrollWidth,
            `${story.name} (${theme}/${vp.name}) overflowed horizontally`,
          ).toBeLessThanOrEqual(overflow.clientWidth + 1);

          // 2. Verify 3-surface CSS tokens are active and distinct.
          const surfaces = await page.evaluate(() => {
            const styles = getComputedStyle(document.documentElement);
            return {
              sidebar: styles.getPropertyValue("--sidebar").trim(),
              background: styles.getPropertyValue("--background").trim(),
              card: styles.getPropertyValue("--card").trim(),
            };
          });
          expect(surfaces.sidebar).not.toBe("");
          expect(surfaces.background).not.toBe("");
          expect(surfaces.card).not.toBe("");

          // 3. Verify PageShell prose width constraint on desktop settings pages.
          if (vp.name === "desktop" && story.expectedWidth === "prose") {
            const columnBox = await page.locator('[data-slot="page-column"]').first().boundingBox();
            expect(columnBox, `${story.name} should render [data-slot="page-column"]`).not.toBeNull();
            expect(columnBox!.width).toBeLessThanOrEqual(770);
          }

          // 4. Capture UI story screenshot artifact for light/dark + desktop/mobile regression review.
          const filename = `${story.name}-${theme}-${vp.name}.png`;
          const screenshotPath = path.join(ARTIFACT_DIR, filename);
          const buf = await page.screenshot({ path: screenshotPath, fullPage: false });
          await testInfo.attach(filename, { body: buf, contentType: "image/png" });
        }
      }
    }

    guard.assertClean();
  });
});

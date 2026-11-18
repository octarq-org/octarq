import { mkdirSync } from "node:fs";
import path from "node:path";
import { test, expect } from "@playwright/test";
import { ADMIN_USER, ADMIN_PASSWORD, signIn, attachPageGuards, expectNoCommentLeak, expectNoI18nLeak } from "./helpers";

const STORIES = [
  { name: "overview", path: "/admin/overview" },
  { name: "settings-appearance", path: "/admin/settings/appearance" },
  { name: "settings-general", path: "/admin/settings/general" },
  { name: "settings-plugins", path: "/admin/settings/plugins" },
  { name: "audit", path: "/admin/audit" },
] as const;

const VIEWPORTS = [
  { name: "desktop", width: 1440, height: 900 },
  { name: "mobile", width: 390, height: 844 },
] as const;

const THEMES = ["light", "dark"] as const;

const ARTIFACT_DIR = path.resolve(process.cwd(), "test-results", "design-system");

test.describe("Design System UI Stories", () => {
  test("3-surface hierarchy, full-width geometry, and light/dark screenshots across viewports", async ({ page }, testInfo) => {
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
        let baselineShellWidth: number | null = null;

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
          expect(surfaces.sidebar).not.toBe(surfaces.background);
          expect(surfaces.background).not.toBe(surfaces.card);

          // 3. Verify PageShell full-width geometry and ShellFooter alignment across all routes.
          const shell = page.locator('[data-slot="page-shell"]').first();
          await expect(shell, `${story.name} should render [data-slot="page-shell"]`).toBeVisible();
          await expect(shell).toHaveAttribute("data-width", "full");
          await expect(page.locator('[data-slot="page-column"]')).toHaveCount(0);

          if (vp.name === "desktop") {
            const shellBox = await shell.boundingBox();
            expect(shellBox, `${story.name} should have a bounding box`).not.toBeNull();
            expect(shellBox!.width).toBeGreaterThan(900);
            if (baselineShellWidth === null) {
              baselineShellWidth = shellBox!.width;
            } else {
              expect(
                Math.abs(shellBox!.width - baselineShellWidth),
                `${story.name} width ${shellBox!.width} diverged from baseline ${baselineShellWidth}`,
              ).toBeLessThanOrEqual(2);
            }

            const footer = page.locator("main footer").first();
            if ((await footer.count()) > 0) {
              const footerBox = await footer.boundingBox();
              expect(footerBox).not.toBeNull();
              expect(Math.abs(footerBox!.width - shellBox!.width)).toBeLessThanOrEqual(2);
            }
          }

          if (vp.name === "desktop" && story.name === "settings-appearance") {
            const renderedSurfaces = await page.evaluate(() => {
              const frame = document.querySelector("aside > div");
              const panel = document.querySelector("aside + div");
              const card = document.querySelector('[data-slot="page-rows"]');
              return {
                frameBg: frame ? getComputedStyle(frame).backgroundColor : "",
                panelBg: panel ? getComputedStyle(panel).backgroundColor : "",
                cardBg: card ? getComputedStyle(card).backgroundColor : "",
              };
            });
            expect(renderedSurfaces.frameBg).not.toBe("");
            expect(renderedSurfaces.panelBg).not.toBe("");
            expect(renderedSurfaces.cardBg).not.toBe("");
            expect(renderedSurfaces.frameBg).not.toBe(renderedSurfaces.panelBg);
            expect(renderedSurfaces.panelBg).not.toBe(renderedSurfaces.cardBg);
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

  test("dual-rail sidebar collapses to 52px area rail with hover flyout", async ({ page }, testInfo) => {
    mkdirSync(ARTIFACT_DIR, { recursive: true });
    const guard = attachPageGuards(page);

    await page.setViewportSize({ width: 1440, height: 900 });
    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.goto("/admin/overview");
    await page.waitForLoadState("networkidle");

    const asideBox = await page.locator("aside").first().boundingBox();
    const railBox = await page.locator('[data-slot="area-rail"]').first().boundingBox();
    const subpanelBox = await page.locator('[data-slot="area-subpanel"]').first().boundingBox();
    expect(asideBox?.width).toBe(252);
    expect(railBox?.width).toBe(52);
    expect(subpanelBox?.width).toBe(200);

    await page.getByRole("button", { name: "Collapse menu" }).click();
    await expect(page.locator('[data-slot="area-subpanel"]')).toHaveCount(0);
    await expect
      .poll(async () => (await page.locator("aside").first().boundingBox())?.width ?? 0)
      .toBe(52);

    await page.getByRole("button", { name: "Settings" }).hover();
    const flyout = page.locator('[data-slot="area-rail-flyout"]');
    await expect(flyout).toBeVisible();
    await expect(flyout.getByRole("link", { name: "Appearance" })).toBeVisible();
    const flyoutBox = await flyout.boundingBox();
    expect(flyoutBox).not.toBeNull();
    expect(flyoutBox!.y).toBeGreaterThanOrEqual(0);
    expect(flyoutBox!.y + flyoutBox!.height).toBeLessThanOrEqual(900);

    const flyoutBuf = await page.screenshot({
      path: path.join(ARTIFACT_DIR, "dual-rail-collapsed-flyout.png"),
      fullPage: false,
    });
    await testInfo.attach("dual-rail-collapsed-flyout.png", { body: flyoutBuf, contentType: "image/png" });

    await page.keyboard.press("Escape");
    await expect(flyout).toHaveCount(0);

    await page.getByRole("button", { name: "Collapse menu" }).click();
    await expect(page.locator('[data-slot="area-subpanel"]')).toBeVisible();

    guard.assertClean();
  });

  test("command palette renders indented collapsible groups", async ({ page }, testInfo) => {
    mkdirSync(ARTIFACT_DIR, { recursive: true });
    const guard = attachPageGuards(page);

    await page.setViewportSize({ width: 1440, height: 900 });
    await signIn(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.goto("/admin/overview");
    await page.waitForLoadState("networkidle");

    await page.keyboard.press("ControlOrMeta+k");
    const groups = page.locator('[data-slot="command-group"]');
    await expect(groups.first()).toBeVisible();
    expect(await groups.count()).toBeGreaterThanOrEqual(2);

    const firstGroupToggle = groups.first().locator('button[aria-expanded]').first();
    await expect(firstGroupToggle).toHaveAttribute("aria-expanded", "true");

    const cmdkBuf = await page.screenshot({
      path: path.join(ARTIFACT_DIR, "cmdk-grouped.png"),
      fullPage: false,
    });
    await testInfo.attach("cmdk-grouped.png", { body: cmdkBuf, contentType: "image/png" });

    await firstGroupToggle.click();
    await expect(firstGroupToggle).toHaveAttribute("aria-expanded", "false");
    await expect(groups.first().locator('[data-slot="command-group-items"]')).toHaveCount(0);

    await page.keyboard.press("Escape");
    guard.assertClean();
  });
});

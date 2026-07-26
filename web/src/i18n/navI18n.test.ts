import { describe, it, expect, beforeEach } from "vitest";
import {
  registerUIPlugin,
  resetRegistry,
  uiMenus,
  uiAreas,
  uiPluginSharedI18n,
  type Lang,
} from "@octarq/plugin-sdk";
import { en } from "./en";
import { zh } from "./zh";
import { es } from "./es";
import { pt } from "./pt";
import { ja } from "./ja";
import { STATIC_AREAS, SETTINGS_AREA } from "../shell/areas";
import mailPlugin from "../plugins/mail";
import linksPlugin from "../plugins/links";
import dnsPlugin from "../plugins/dns";
import auditPlugin from "../plugins/core/audit";
import abusePlugin from "../plugins/core/abuse";
import helpPlugin from "../plugins/core/help";

const LOCALES: Lang[] = ["en", "zh", "es", "pt", "ja"];

const HOST_RESOURCES: Record<Lang, Record<string, unknown>> = {
  en,
  zh,
  es,
  pt,
  ja,
};

// Helper: deep-get a dotted key path like "groups.Communication" or "nav.overview"
function getDottedKey(obj: unknown, path: string): unknown {
  if (obj == null || typeof obj !== "object") return undefined;
  const parts = path.split(".");
  let cur: any = obj;
  for (const part of parts) {
    if (cur == null || typeof cur !== "object") return undefined;
    cur = cur[part];
  }
  return cur;
}

describe("Navigation & Category Group i18n Guard Test", () => {
  beforeEach(() => {
    resetRegistry();
    // Register all core / default frontend plugins so full seam is reachable
    registerUIPlugin(mailPlugin);
    registerUIPlugin(linksPlugin);
    registerUIPlugin(dnsPlugin);
    registerUIPlugin(auditPlugin);
    registerUIPlugin(abusePlugin);
    registerUIPlugin(helpPlugin);
  });

  it("ensures every category group heading resolves to an explicit key in en, zh, es, pt, ja", () => {
    // Collect all group category labels from static areas, settings, plugin menus & plugin areas
    const categories = new Set<string>();

    for (const area of [...STATIC_AREAS, SETTINGS_AREA]) {
      for (const group of area.groups) {
        if (group.label) categories.add(group.label);
      }
    }

    for (const menu of uiMenus()) {
      if (menu.category) categories.add(menu.category);
    }

    for (const area of uiAreas()) {
      if (area.groups) {
        for (const g of area.groups) categories.add(g);
      }
    }

    const sharedDicts = uiPluginSharedI18n();

    for (const locale of LOCALES) {
      const hostDict = HOST_RESOURCES[locale] || {};
      const sharedDict = sharedDicts[locale] || {};

      for (const category of categories) {
        // Skip "footer" / "resources" which is placement metadata rather than a group heading
        if (category.toLowerCase() === "footer" || category.toLowerCase() === "resources") continue;

        const hostVal = getDottedKey(hostDict, `groups.${category}`);
        const sharedVal = getDottedKey(sharedDict, `groups.${category}`);

        const resolved = hostVal ?? sharedVal;

        expect(
          typeof resolved === "string" && (resolved as string).trim().length > 0,
          `Locale "${locale}" is missing translation for category group "${category}" (expected key: groups.${category})`,
        ).toBe(true);
      }
    }
  });

  it("ensures every navigation menu item id resolves to an explicit key in en, zh, es, pt, ja", () => {
    const menuIds = new Set<string>();

    for (const area of [...STATIC_AREAS, SETTINGS_AREA]) {
      for (const group of area.groups) {
        for (const item of group.items) {
          if (item.id) menuIds.add(item.id);
        }
      }
    }

    for (const menu of uiMenus()) {
      if (menu.id) menuIds.add(menu.id);
    }

    const sharedDicts = uiPluginSharedI18n();

    for (const locale of LOCALES) {
      const hostDict = HOST_RESOURCES[locale] || {};
      const sharedDict = sharedDicts[locale] || {};

      for (const id of menuIds) {
        const hostVal = getDottedKey(hostDict, `nav.${id}`);
        const sharedVal = getDottedKey(sharedDict, `nav.${id}`);

        const resolved = hostVal ?? sharedVal;

        expect(
          typeof resolved === "string" && (resolved as string).trim().length > 0,
          `Locale "${locale}" is missing translation for menu item "${id}" (expected key: nav.${id})`,
        ).toBe(true);
      }
    }
  });
});

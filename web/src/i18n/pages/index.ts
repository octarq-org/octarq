// Per-page i18n namespaces. Each page owns one file exporting `{ en, zh }` so
// pages can be translated independently without touching a shared dictionary.
import { overview } from "./overview";
import { personal } from "./personal";
import { settings } from "./settings";
// Note: Feature namespaces (links, domains, mail, licenses, etc.) are owned by
// their UIPlugin and injected via UIPlugin.i18n when composed into the build.
import { abuse } from "./abuse";
import { audit } from "./audit";
import { invite } from "./invite";
import { uiCommon } from "./uiCommon";

const NS = {
  overview,
  personal,
  settings,
  abuse,
  audit,
  invite,
  uiCommon,
};

export const pagesEn = Object.fromEntries(Object.entries(NS).map(([k, v]) => [k, v.en]));
export const pagesZh = Object.fromEntries(Object.entries(NS).map(([k, v]) => [k, v.zh]));

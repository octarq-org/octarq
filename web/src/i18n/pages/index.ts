// Per-page i18n namespaces. Each page owns one file exporting `{ en, zh }` so
// pages can be translated independently without touching a shared dictionary.
import { overview } from "./overview";
import { links } from "./links";
import { domains } from "./domains";
import { mail } from "./mail";
import { personal } from "./personal";
import { settings } from "./settings";
// Note: the Pro namespaces (licenses, inboxAi, llmProviders, vps, sshKeys,
// finance, storefront, billing) are owned by their UIPlugin (octarq-pro
// packages) and injected via UIPlugin.i18n only when composed into the Pro
// build. The OSS bundle never ships them. Audit is a *core* feature (its backend
// is core, /api/audit), so its namespace lives here.
import { abuse } from "./abuse";
import { audit } from "./audit";
import { invite } from "./invite";
import { uiCommon } from "./uiCommon";
import { portal } from "./portal";

const NS = {
  overview,
  links,
  domains,
  mail,
  personal,
  settings,
  abuse,
  audit,
  invite,
  uiCommon,
  portal,
};

export const pagesEn = Object.fromEntries(Object.entries(NS).map(([k, v]) => [k, v.en]));
export const pagesZh = Object.fromEntries(Object.entries(NS).map(([k, v]) => [k, v.zh]));

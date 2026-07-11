// Per-page i18n namespaces. Each page owns one file exporting `{ en, zh }` so
// pages can be translated independently without touching a shared dictionary.
import { overview } from "./overview";
import { links } from "./links";
import { domains } from "./domains";
import { mail } from "./mail";
import { personal } from "./personal";
import { settings } from "./settings";
// Note: the Pro namespaces (licenses, inboxAi, llmProviders, vps, sshKeys,
// finance, storefront, billing, audit) moved out of this central bundle — each
// is now owned by its UIPlugin (web/src/plugins/*) and injected via
// UIPlugin.i18n only when that plugin is composed in (the Pro build). The OSS
// bundle never ships them.
import { abuse } from "./abuse";
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
  invite,
  uiCommon,
  portal,
};

export const pagesEn = Object.fromEntries(Object.entries(NS).map(([k, v]) => [k, v.en]));
export const pagesZh = Object.fromEntries(Object.entries(NS).map(([k, v]) => [k, v.zh]));

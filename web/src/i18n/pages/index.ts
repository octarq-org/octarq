// Per-page i18n namespaces. Each page owns one file exporting `{ en, zh }` so
// pages can be translated independently without touching a shared dictionary.
import { overview } from "./overview";
import { links } from "./links";
import { domains } from "./domains";
import { mail } from "./mail";
import { inboxAi } from "./inboxAi";
import { vps } from "./vps";
import { sshKeys } from "./sshKeys";
import { finance } from "./finance";
import { storefront } from "./storefront";
import { personal } from "./personal";
import { settings } from "./settings";
import { billing } from "./billing";
// Note: the `licenses` namespace moved out of this central bundle — it is now
// owned by the licenses UIPlugin (web/src/plugins/licenses) and injected via
// UIPlugin.i18n when that plugin is composed in.
import { audit } from "./audit";
import { abuse } from "./abuse";
import { invite } from "./invite";
import { llmProviders } from "./llmProviders";
import { uiCommon } from "./uiCommon";
import { portal } from "./portal";

const NS = {
  overview,
  links,
  domains,
  mail,
  inboxAi,
  vps,
  sshKeys,
  finance,
  storefront,
  personal,
  settings,
  billing,
  audit,
  abuse,
  invite,
  llmProviders,
  uiCommon,
  portal,
};

export const pagesEn = Object.fromEntries(Object.entries(NS).map(([k, v]) => [k, v.en]));
export const pagesZh = Object.fromEntries(Object.entries(NS).map(([k, v]) => [k, v.zh]));

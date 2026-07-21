// Host adapter over the SDK's i18n (@octarq/plugin-sdk). The SDK owns the
// mechanism (context, useTranslation, plugin-namespace merge); this module owns
// the app's *content* — it builds the shell + per-page resource dictionaries
// and mounts the SDK provider with them. Re-exporting the SDK hooks keeps every
// existing `import { useTranslation } from "../i18n"` working AND puts core
// pages on the same context as plugin packages (so a plugin's useTranslation and
// a core page's resolve identically).
//
// Imported by source path (not the `@octarq/plugin-sdk` alias, which resolves
// to the app-side facade) so the dependency points app → package, never back.
import { ReactNode } from "react";
import {
  I18nProvider as SdkI18nProvider,
  type Resources,
} from "../../../packages/plugin-sdk/src";
import { en } from "./en";
import { zh } from "./zh";
import { pagesEn, pagesZh } from "./pages";

export type { Lang, TFunc } from "../../../packages/plugin-sdk/src";
export { useI18n, useTranslation, LANGS } from "../../../packages/plugin-sdk/src";

// Shell resources (en/zh) plus per-page namespaces from ./pages. Plugin
// namespaces (UIPlugin.i18n) are folded in by the SDK provider at render time.
const RESOURCES: Resources = {
  en: { ...en, ...pagesEn },
  zh: { ...zh, ...pagesZh },
};

export function I18nProvider({ children }: { children: ReactNode }) {
  return <SdkI18nProvider resources={RESOURCES}>{children}</SdkI18nProvider>;
}

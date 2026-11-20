import { ReactNode } from "react";
import {
  I18nProvider as SdkI18nProvider,
  type Resources,
} from "../../../packages/plugin-sdk/src";
import { en } from "./en";
import { zh } from "./zh";
import { es } from "./es";
import { pt } from "./pt";
import { ja } from "./ja";
import { pagesEn, pagesZh, pagesEs, pagesPt, pagesJa } from "./pages";

export type { Lang, TFunc } from "../../../packages/plugin-sdk/src";
export { useI18n, useTranslation, LANGS } from "../../../packages/plugin-sdk/src";

const RESOURCES: Resources = {
  en: { ...en, ...pagesEn },
  zh: { ...zh, ...pagesZh },
  es: { ...es, ...pagesEs },
  pt: { ...pt, ...pagesPt },
  ja: { ...ja, ...pagesJa },
};

export function I18nProvider({ children }: { children: ReactNode }) {
  return <SdkI18nProvider resources={RESOURCES}>{children}</SdkI18nProvider>;
}

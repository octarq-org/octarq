import { createContext, useContext, useEffect, useMemo, useState, ReactNode } from "react";
import { en } from "./en";
import { zh } from "./zh";
import { pagesEn, pagesZh } from "./pages";
import { uiPluginI18n } from "../plugin-sdk";

export type Lang = "en" | "zh";

export const LANGS: { code: Lang; label: string }[] = [
  { code: "en", label: "English" },
  { code: "zh", label: "中文" },
];

// Shell resources (en/zh) plus per-page namespaces merged in from ./pages.
const RESOURCES: Record<Lang, Record<string, any>> = {
  en: { ...en, ...pagesEn },
  zh: { ...zh, ...pagesZh },
};

// Detect the initial language: an explicit choice wins, otherwise fall back to
// the browser's preference (any zh-* locale → Chinese), else English.
function detectLang(): Lang {
  try {
    const saved = localStorage.getItem("lang");
    if (saved === "en" || saved === "zh") return saved;
  } catch { /* ignore */ }
  const nav = (navigator.languages?.[0] || navigator.language || "en").toLowerCase();
  return nav.startsWith("zh") ? "zh" : "en";
}

// Walk a nested resource object by a dotted key path (e.g. "nav.areas.commerce").
function lookup(dict: Record<string, any>, key: string): string | undefined {
  let cur: any = dict;
  for (const seg of key.split(".")) {
    if (cur == null || typeof cur !== "object") return undefined;
    cur = cur[seg];
  }
  return typeof cur === "string" ? cur : undefined;
}

function interpolate(s: string, vars?: Record<string, string | number>): string {
  if (!vars) return s;
  return s.replace(/\{\{(\w+)\}\}/g, (_, k) => (k in vars ? String(vars[k]) : `{{${k}}}`));
}

export type TFunc = (key: string, fallbackOrVars?: string | Record<string, string | number>, vars?: Record<string, string | number>) => string;

interface I18nCtx {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: TFunc;
}

const Ctx = createContext<I18nCtx | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectLang);

  useEffect(() => {
    try { localStorage.setItem("lang", lang); } catch { /* ignore */ }
    document.documentElement.lang = lang;
  }, [lang]);

  const value = useMemo<I18nCtx>(() => {
    // Merge namespaces contributed by composed frontend plugins (UIPlugin.i18n)
    // over the core resources. Done here at render time — not at module eval —
    // so it is independent of whether the plugin injection module happened to
    // evaluate before this one.
    const pluginNs = uiPluginI18n();
    const dict: Record<Lang, Record<string, any>> = {
      en: { ...RESOURCES.en, ...pluginNs.en },
      zh: { ...RESOURCES.zh, ...pluginNs.zh },
    };
    // t(key), t(key, fallback), t(key, vars), or t(key, fallback, vars).
    const t: TFunc = (key, fallbackOrVars, vars) => {
      let fallback: string | undefined;
      let interp = vars;
      if (typeof fallbackOrVars === "string") fallback = fallbackOrVars;
      else if (fallbackOrVars) interp = fallbackOrVars;
      const hit = lookup(dict[lang], key) ?? lookup(dict.en, key) ?? fallback ?? key;
      return interpolate(hit, interp);
    };
    return { lang, setLang: setLangState, t };
  }, [lang]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useI18n(): I18nCtx {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}

// Convenience hook mirroring react-i18next's shape for ergonomics.
export function useTranslation(): { t: TFunc; lang: Lang; setLang: (l: Lang) => void } {
  return useI18n();
}

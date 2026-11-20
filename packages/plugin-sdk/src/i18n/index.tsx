import { createContext, useContext, useEffect, useMemo, useState, ReactNode } from "react";
import { uiPluginI18n, uiPluginSharedI18n } from "../contract";

export type Lang = "en" | "zh" | "es" | "pt" | "ja";

export const LANGS: { code: Lang; label: string }[] = [
  { code: "en", label: "English" },
  { code: "zh", label: "中文" },
  { code: "es", label: "Español" },
  { code: "pt", label: "Português" },
  { code: "ja", label: "日本語" },
];

export type Resources = Partial<Record<Lang, Record<string, unknown>>>;

export function detectLang(): Lang {
  try {
    const saved = localStorage.getItem("lang");
    if (saved === "en" || saved === "zh" || saved === "es" || saved === "pt" || saved === "ja") return saved as Lang;
  } catch {
  }
  const nav = (navigator.languages?.[0] || navigator.language || "en").toLowerCase();
  if (nav.startsWith("zh")) return "zh";
  if (nav.startsWith("es")) return "es";
  if (nav.startsWith("pt")) return "pt";
  if (nav.startsWith("ja")) return "ja";
  return "en";
}

function overlay(
  base: Record<string, unknown>,
  top: Record<string, unknown>,
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...base };
  for (const [k, v] of Object.entries(top)) {
    const cur = out[k];
    out[k] =
      cur != null && typeof cur === "object" && v != null && typeof v === "object"
        ? overlay(cur as Record<string, unknown>, v as Record<string, unknown>)
        : v;
  }
  return out;
}

function lookup(dict: Record<string, unknown>, key: string): string | undefined {
  let cur: unknown = dict;
  for (const seg of key.split(".")) {
    if (cur == null || typeof cur !== "object") return undefined;
    cur = (cur as Record<string, unknown>)[seg];
  }
  return typeof cur === "string" ? cur : undefined;
}

function interpolate(s: string, vars?: Record<string, string | number>): string {
  if (!vars) return s;
  return s.replace(/\{\{(\w+)\}\}/g, (_, k) => (k in vars ? String(vars[k]) : `{{${k}}}`));
}

export type TFunc = (
  key: string,
  fallbackOrVars?: string | Record<string, string | number>,
  vars?: Record<string, string | number>,
) => string;

interface I18nCtx {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: TFunc;
}

const Ctx = createContext<I18nCtx | null>(null);

export function I18nProvider({ resources = {}, children }: { resources?: Resources; children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectLang);

  useEffect(() => {
    try {
      localStorage.setItem("lang", lang);
    } catch {
    }
    document.documentElement.lang = lang;
  }, [lang]);

  const value = useMemo<I18nCtx>(() => {
    const pluginNs = uiPluginI18n();
    const shared = uiPluginSharedI18n();

    const buildLangDict = (l: Lang): Record<string, unknown> => {
      const s = shared[l] || {};
      const r = (resources && resources[l]) || {};
      const p = pluginNs[l] || {};
      return overlay(s, { ...r, ...p });
    };

    const dict: Record<Lang, Record<string, unknown>> = {
      en: buildLangDict("en"),
      zh: buildLangDict("zh"),
      es: buildLangDict("es"),
      pt: buildLangDict("pt"),
      ja: buildLangDict("ja"),
    };

    const t: TFunc = (key, fallbackOrVars, vars) => {
      let fallback: string | undefined;
      let interp = vars;
      if (typeof fallbackOrVars === "string") fallback = fallbackOrVars;
      else if (fallbackOrVars) interp = fallbackOrVars;
      const currentDict = dict[lang] || {};
      const enDict = dict.en || {};
      const hit = lookup(currentDict, key) ?? lookup(enDict, key) ?? fallback ?? key;
      return interpolate(hit, interp);
    };
    return { lang, setLang: setLangState, t };
  }, [lang, resources]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useI18n(): I18nCtx {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}

export function useTranslation(): { t: TFunc; lang: Lang; setLang: (l: Lang) => void } {
  return useI18n();
}

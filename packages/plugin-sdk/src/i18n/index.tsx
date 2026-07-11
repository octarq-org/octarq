// @octarq-org/plugin-sdk/i18n — the self-contained i18n surface.
//
// The SDK owns the i18n *mechanism* (context, provider, lookup, interpolation);
// the host app owns the *content*. The app mounts <I18nProvider resources={…}>
// feeding its merged shell/page translations, and the SDK folds in every
// composed plugin's namespace (UIPlugin.i18n via uiPluginI18n()) on top. Plugin
// pages then call useTranslation() from the SDK and read the same context —
// which is what lets a plugin ship as an independent package without importing
// anything app-internal.
import { createContext, useContext, useEffect, useMemo, useState, ReactNode } from "react";
import { uiPluginI18n } from "../contract";

export type Lang = "en" | "zh";

export const LANGS: { code: Lang; label: string }[] = [
  { code: "en", label: "English" },
  { code: "zh", label: "中文" },
];

// A per-language resource dictionary (nested namespaces of strings).
export type Resources = Record<Lang, Record<string, unknown>>;

function detectLang(): Lang {
  try {
    const saved = localStorage.getItem("lang");
    if (saved === "en" || saved === "zh") return saved;
  } catch {
    /* ignore */
  }
  const nav = (navigator.languages?.[0] || navigator.language || "en").toLowerCase();
  return nav.startsWith("zh") ? "zh" : "en";
}

// Walk a nested resource object by a dotted key path (e.g. "nav.areas.commerce").
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

// I18nProvider is mounted by the host app with its own resource dictionaries.
// The SDK merges composed-plugin namespaces over them at render time (so it is
// independent of plugin-module eval order).
export function I18nProvider({ resources, children }: { resources: Resources; children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectLang);

  useEffect(() => {
    try {
      localStorage.setItem("lang", lang);
    } catch {
      /* ignore */
    }
    document.documentElement.lang = lang;
  }, [lang]);

  const value = useMemo<I18nCtx>(() => {
    const pluginNs = uiPluginI18n();
    const dict: Resources = {
      en: { ...resources.en, ...pluginNs.en },
      zh: { ...resources.zh, ...pluginNs.zh },
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
    // resources is a stable module-level object on the host side; re-run on lang.
  }, [lang, resources]);

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

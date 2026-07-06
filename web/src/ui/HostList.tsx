import { ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { twMerge } from "tailwind-merge";
import { motion } from "framer-motion";
import { HostEntry } from "../api";
import { useAppName } from "../brand";
import { useTranslation } from "../i18n";

export function HostList({
  hosts,
  onChange,
  suggestions = [],
  placeholder,
  baseDomain,
  emptyText,
}: {
  hosts: HostEntry[];
  onChange: (hosts: HostEntry[]) => void;
  suggestions?: string[];
  placeholder?: string;
  baseDomain?: string;
  emptyText?: string;
}) {
  const [draft, setDraft] = useState("");
  const { t } = useTranslation();

  function resolve(raw: string): string {
    const v = raw.trim().toLowerCase();
    if (!v) return v;
    if (baseDomain && !v.includes(".")) return `${v}.${baseDomain}`;
    return v;
  }

  function add(h: string) {
    const v = resolve(h);
    if (v && !hosts.some((x) => x.host === v)) {
      onChange([...hosts, { host: v, enabled: true }]);
    }
    setDraft("");
  }

  const freshSuggestions = suggestions.filter((s) => !hosts.some((x) => x.host === s));

  return (
    <div className="rounded-xl border border-white/10 bg-white/[0.03] p-2.5">
      <div className="mb-2 flex flex-wrap gap-1.5">
        {hosts.length === 0 ? (
          <span className="text-xs text-white/40">{emptyText ?? t("uiCommon.hostListEmpty")}</span>
        ) : (
          hosts.map((h) => (
            <span
              key={h.host}
              className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-sm border transition-colors ${
                h.enabled
                  ? "bg-indigo-500/15 text-indigo-200 border-indigo-500/25"
                  : "bg-white/5 text-white/35 border-white/10 line-through"
              }`}
            >
              <button
                type="button"
                className={`cursor-pointer hover:text-white text-xs ${h.enabled ? "text-indigo-400" : "text-white/35"}`}
                title={h.enabled ? t("uiCommon.disableHost") : t("uiCommon.enableHost")}
                onClick={() =>
                  onChange(hosts.map((x) => (x.host === h.host ? { ...x, enabled: !x.enabled } : x)))
                }
              >
                {h.enabled ? "●" : "○"}
              </button>
              <span className="select-none">{h.host}</span>
              <button
                type="button"
                className="text-white/30 hover:text-rose-400 ml-0.5"
                title={t("uiCommon.remove")}
                onClick={() => onChange(hosts.filter((x) => x.host !== h.host))}
              >
                ✕
              </button>
            </span>
          ))
        )}
      </div>
      <div className="flex gap-2">
        <div className="relative flex-1">
          <input
            className="input w-full"
            value={draft}
            placeholder={placeholder ?? t("uiCommon.hostPlaceholder")}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") { e.preventDefault(); add(draft); }
            }}
          />
          {baseDomain && draft && !draft.includes(".") && (
            <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-white/35">
              → {draft.trim().toLowerCase()}.{baseDomain}
            </span>
          )}
        </div>
        <button className="btn-primary shrink-0" type="button" disabled={!draft.trim()} onClick={() => add(draft)}>
          {t("uiCommon.addHost")}
        </button>
      </div>
      {freshSuggestions.length > 0 && (
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-white/40">{t("uiCommon.quickAdd")}</span>
          {freshSuggestions.map((s) => (
            <button
              key={s}
              type="button"
              className="rounded-lg border border-indigo-500/35 px-2 py-0.5 text-xs text-indigo-300 transition hover:bg-indigo-500/10"
              onClick={() => add(s)}
            >
              + {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Guide ───────────────────────────────────────────────────────────────────


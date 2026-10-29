import { ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { twMerge } from "tailwind-merge";
import { motion } from "framer-motion";
import { HostEntry } from "../api";
import { useAppName } from "../brand";
import { useTranslation } from "../i18n";
import { Input } from "@octarq/plugin-sdk";

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
    <div className="rounded-xl border border-border bg-well p-2.5">
      <div className="mb-2 flex flex-wrap gap-1.5">
        {hosts.length === 0 ? (
          <span className="text-xs text-muted-foreground">{emptyText ?? t("uiCommon.hostListEmpty")}</span>
        ) : (
          hosts.map((h) => (
            <span
              key={h.host}
              className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-sm border transition-colors ${
                h.enabled
                  ? "bg-accent-soft text-accent-fg border-accent-border"
                  : "bg-well text-muted-foreground border-border line-through"
              }`}
            >
              <button
                type="button"
                className={`cursor-pointer hover:text-foreground text-xs ${h.enabled ? "text-accent-fg" : "text-muted-foreground"}`}
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
                className="text-muted-foreground hover:text-danger-fg ml-0.5"
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
          <Input
            value={draft}
            placeholder={placeholder ?? t("uiCommon.hostPlaceholder")}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") { e.preventDefault(); add(draft); }
            }}
          />
          {baseDomain && draft && !draft.includes(".") && (
            <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-muted-foreground">
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
          <span className="text-xs text-muted-foreground">{t("uiCommon.quickAdd")}</span>
          {freshSuggestions.map((s) => (
            <button
              key={s}
              type="button"
              className="rounded-lg border border-accent-border px-2 py-0.5 text-xs text-accent-fg transition hover:bg-accent-soft"
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



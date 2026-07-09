import { ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { twMerge } from "tailwind-merge";
import { motion } from "framer-motion";
import { HostEntry } from "../api";
import { useAppName } from "../brand";
import { useTranslation } from "../i18n";

export function AreaChart({
  series,
  height = 120,
}: {
  series: { key: string; count: number }[];
  height?: number;
}) {
  const { t } = useTranslation();
  if (!series || !series.length)
    return <div className="grid h-28 place-items-center text-sm text-white/50">{t("uiCommon.noDataYet")}</div>;

  const w = 600;
  const h = height;
  const pad = 6;
  const max = Math.max(...series.map((s) => s.count), 1);
  const n = series.length;
  const x = (i: number) => (n === 1 ? w / 2 : pad + (i * (w - 2 * pad)) / (n - 1));
  const y = (v: number) => h - pad - (v / max) * (h - 2 * pad);
  const pts = series.map((s, i) => `${x(i)},${y(s.count)}`);
  const line = `M ${pts.join(" L ")}`;
  const area = `${line} L ${x(n - 1)},${h - pad} L ${x(0)},${h - pad} Z`;

  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="w-full" preserveAspectRatio="none" style={{ height }}>
      <defs>
        <linearGradient id="octarq-area" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="rgb(99 102 241)" stopOpacity="0.5" />
          <stop offset="100%" stopColor="rgb(99 102 241)" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill="url(#octarq-area)" />
      <path d={line} fill="none" stroke="rgb(129 140 248)" strokeWidth="2" vectorEffect="non-scaling-stroke" />
      {series.map((s, i) => (
        <circle key={i} cx={x(i)} cy={y(s.count)} r="2.5" fill="rgb(129 140 248)">
          <title>{`${s.key}: ${s.count}`}</title>
        </circle>
      ))}
    </svg>
  );
}

// ─── BarList ──────────────────────────────────────────────────────────────────

export function BarList({
  rows,
  empty = "—",
}: {
  rows: { key: string; count: number }[] | null;
  empty?: string;
}) {
  const { t } = useTranslation();
  if (!rows || rows.length === 0) return <p className="text-sm text-white/50">{empty}</p>;
  const max = Math.max(...rows.map((r) => r.count), 1);
  return (
    <div className="space-y-1.5">
      {rows.map((r) => (
        <div key={r.key} className="flex items-center gap-2 text-sm">
          <span className="w-24 truncate text-white/70">{r.key || t("uiCommon.direct")}</span>
          <div className="h-2 flex-1 overflow-hidden rounded-full bg-white/8">
            <div
              className="h-full rounded-full bg-indigo-500/60"
              style={{ width: `${(r.count / max) * 100}%` }}
            />
          </div>
          <span className="w-8 text-right text-white/40">{r.count}</span>
        </div>
      ))}
    </div>
  );
}

// ─── timeAgo ──────────────────────────────────────────────────────────────────


import { ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { twMerge } from "tailwind-merge";
import { motion } from "framer-motion";
import { HostEntry } from "../api";
import { useAppName } from "../brand";
import { useTranslation } from "../i18n";
import { GlassCard, ProPill, Button, TIER_LABEL } from "./primitives";

export function LockedFeature({
  status,
  tier = "pro",
  feature,
  description,
  perks,
  icon,
  pricingHref,
}: {
  status: number;        // 402 (unlicensed) or 404 (plugin not in this build) → upsell
  tier?: "pro" | "elite";
  feature: string;       // e.g. "VPS Infrastructure"
  description?: string;  // one line on what the feature does
  perks?: string[];      // what unlocking grants
  icon?: ReactNode;      // lucide icon node from the caller (keeps ui.tsx icon-free)
  pricingHref?: string;  // optional "compare plans" link to the landing page
}) {
  // Both "unlicensed" (402) and "not built into this installation" (404) are
  // gated Pro states — show one unified upsell mask. Only genuinely unexpected
  // failures fall through to the neutral message.
  const locked = status === 402 || status === 404;
  const label = TIER_LABEL[tier];
  const appName = useAppName();
  const { t } = useTranslation();

  return (
    <GlassCard
      strong
      className="mx-auto mt-12 flex max-w-md flex-col items-center gap-5 px-6 py-14 text-center"
    >
      <div
        className={twMerge(
          "flex h-14 w-14 items-center justify-center rounded-2xl",
          locked
            ? "bg-gradient-to-br from-indigo-500/20 to-violet-500/20 text-violet-300 ring-1 ring-inset ring-violet-400/25"
            : "bg-rose-500/10 text-rose-400",
        )}
      >
        {icon}
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-center gap-2">
          <h2 className="text-xl font-bold text-white">{feature}</h2>
          {locked && <ProPill>{label}</ProPill>}
        </div>
        <p className="text-sm leading-relaxed text-white/50">
          {locked
            ? <>{t("uiCommon.lockedIntroPre")}<span className="font-medium text-violet-200">{appName} {label}</span>{t("uiCommon.lockedIntroPost")}{description ? ` ${description}` : ""}</>
            : t("uiCommon.notAvailable", { feature })}
        </p>
      </div>

      {locked && perks && perks.length > 0 && (
        <ul className="w-full space-y-1.5 text-left">
          {perks.map((p) => (
            <li key={p} className="flex items-start gap-2 text-sm text-white/65">
              <span className="mt-1 h-1.5 w-1.5 flex-none rounded-full bg-violet-400/70" />
              {p}
            </li>
          ))}
        </ul>
      )}

      {locked && (
        <div className="flex flex-col items-stretch gap-2 pt-1 sm:flex-row">
          <Button variant="primary" onClick={() => (window.location.href = "/admin/settings/license")}>
            {t("uiCommon.upgradeTo", { tier: label })}
          </Button>
          {pricingHref && (
            <a
              href={pricingHref}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center justify-center rounded-xl px-3.5 py-2 text-sm font-medium text-white/65 transition-colors hover:bg-white/5 hover:text-white"
            >
              {t("uiCommon.comparePlans")}
            </a>
          )}
        </div>
      )}
    </GlassCard>
  );
}

// ─── StatCard ─────────────────────────────────────────────────────────────────


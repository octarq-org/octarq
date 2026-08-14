// The gated-state UI: LockedFeature (the upsell mask for 402 unlicensed / 404
// plugin-not-in-this-build) and LockedFallback (a one-liner convenience with a
// default icon). Moved into the published package — driven by the SDK's own
// i18n + brand context and primitives — so an independent plugin can render the
// locked state without reaching into the host app.
import { ReactNode } from "react";
import { twMerge } from "tailwind-merge";
import { useTranslation } from "../i18n";
import { useAppName } from "../brand";
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
  status: number; // 402 (unlicensed) or 404 (plugin not in this build) → upsell
  tier?: "pro" | "elite";
  feature: string; // e.g. "VPS Infrastructure"
  description?: string; // one line on what the feature does
  perks?: string[]; // what unlocking grants
  icon?: ReactNode; // icon node from the caller (keeps the package icon-lib-free)
  pricingHref?: string; // optional "compare plans" link to the landing page
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
            ? "bg-gradient-to-br from-[color:color-mix(in_srgb,var(--accent-indigo)_20%,transparent)] to-[color:color-mix(in_srgb,var(--accent-violet)_20%,transparent)] text-accent-fg ring-1 ring-inset ring-[color:color-mix(in_srgb,var(--primary)_25%,transparent)]"
            : "bg-rose-500/10 text-danger-fg", /* ui-color-ok */
        )}
      >
        {icon ?? <DefaultLockIcon />}
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-center gap-2">
          <h2 className="text-xl font-bold text-foreground">{feature}</h2>
          {locked && <ProPill>{label}</ProPill>}
        </div>
        <p className="text-sm leading-relaxed text-foreground/50">
          {locked ? (
            <>
              {t("uiCommon.lockedIntroPre")}
              <span className="font-medium text-accent-fg">
                {appName} {label}
              </span>
              {t("uiCommon.lockedIntroPost")}
              {description ? ` ${description}` : ""}
            </>
          ) : (
            t("uiCommon.notAvailable", { feature })
          )}
        </p>
      </div>

      {locked && perks && perks.length > 0 && (
        <ul className="w-full space-y-1.5 text-left">
          {perks.map((p) => (
            <li key={p} className="flex items-start gap-2 text-sm text-foreground/65">
              <span className="mt-1 h-1.5 w-1.5 flex-none rounded-full bg-violet-400/70" /* ui-color-ok */ />
              {p}
            </li>
          ))}
        </ul>
      )}

      {locked && (
        <div className="flex flex-col items-stretch gap-2 pt-1 sm:flex-row">
          <Button
            variant="primary"
            onClick={() => (window.location.href = "/admin/license")}
          >
            {t("uiCommon.upgradeTo", { tier: label })}
          </Button>
          {pricingHref && (
            <a
              href={pricingHref}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center justify-center rounded-xl px-3.5 py-2 text-sm font-medium text-foreground/65 transition-colors hover:bg-foreground/5 hover:text-foreground"
            >
              {t("uiCommon.comparePlans")}
            </a>
          )}
        </div>
      )}
    </GlassCard>
  );
}

// LockedFallback is the convenience the frontend plugin contract points at
// (UIPlugin.lockedFallback): a LockedFeature preset with a default key icon so a
// plugin page can degrade in one line — <LockedFallback status={err.status}
// feature="…" />.
export function LockedFallback({
  status,
  feature,
  description,
  perks,
  tier = "pro",
  pricingHref,
}: {
  status: number;
  feature: string;
  description?: string;
  perks?: string[];
  tier?: "pro" | "elite";
  pricingHref?: string;
}) {
  return (
    <LockedFeature
      status={status}
      tier={tier}
      feature={feature}
      description={description}
      perks={perks}
      pricingHref={pricingHref}
    />
  );
}

// A small inline key glyph — keeps the package free of an icon-library dep while
// still giving the locked state a sensible default visual.
function DefaultLockIcon() {
  return (
    <svg
      width="28"
      height="28"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m7 11 8.5-8.5a3.54 3.54 0 1 1 5 5L11 16" />
      <circle cx="6.5" cy="16.5" r="3.5" />
    </svg>
  );
}

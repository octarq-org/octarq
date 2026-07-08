// The shared-UI surface a plugin may import from `@octarq-org/plugin-sdk`.
//
// It unions the package's pure component set with the handful of app-COUPLED
// components that can't live in the package because they read the app's React
// context (i18n / brand):
//   - package (packages/plugin-sdk): GlassCard, Button, Badge, Modal, Toggle,
//     Field, Empty, PageHeader, ScreenWrap, StatCard, ProPill, TIER_LABEL, and
//     the new Input/Textarea/Select/Tabs/Tooltip/Table/Skeleton set.
//   - app: `Code` (uses `useTranslation` for its copy affordance), `Guide`
//     (kept app-side alongside Code), and `LockedFeature` (uses the app's brand
//     + i18n). `useTranslation` itself is re-exported so plugin pages translate
//     through the same provider.
//
// The package is imported by SOURCE PATH, not by the `@octarq-org/plugin-sdk` name,
// which is aliased back to this facade.
export * from "../../../packages/plugin-sdk/src/ui";

export { useTranslation } from "../i18n";
export { Code, Guide, LockedFeature } from "../ui";

import { KeyRound } from "lucide-react";
import { LockedFeature } from "../ui";

// The convenience component for the gated 402 (unlicensed) / 404 (plugin not in
// this build) states, matching octarq's convention: 402 → upsell, 404 → neutral
// note. It wraps the app's `LockedFeature` with a default icon so a plugin page
// can degrade in one line:
//
//   if (err) return <ScreenWrap><LockedFallback status={err.status} feature="…" /></ScreenWrap>;
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
      icon={<KeyRound className="h-7 w-7" />}
      pricingHref={pricingHref}
    />
  );
}

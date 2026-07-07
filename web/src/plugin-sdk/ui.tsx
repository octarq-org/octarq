// The stable shared-UI surface a plugin is allowed to import. This is the
// curated subset of the app's `../ui` barrel we commit to keeping stable for
// plugin authors — plugins import from here (`@led/plugin-sdk`), never from
// `../ui` directly, so the app's internal component churn can't break them.
//
// Re-exported (not re-implemented) so there is a single source of truth for
// each component; when this becomes a published package, this file becomes its
// public component export.
export {
  GlassCard,
  Badge,
  Button,
  ProPill,
  StatCard,
  PageHeader,
  ScreenWrap,
  Modal,
  Field,
  Empty,
  Toggle,
  Guide,
  Code,
  TIER_LABEL,
  LockedFeature,
} from "../ui";

// Also re-export the i18n hook so plugin pages translate through the same
// provider and can register their own namespace via `UIPlugin.i18n`.
export { useTranslation } from "../i18n";

import { KeyRound } from "lucide-react";
import { LockedFeature } from "../ui";

// The SDK-provided component for the gated 402/404 states, matching led's
// convention: 402 (unlicensed) → upsell, 404 (plugin not in this build) →
// neutral note. It wraps the app's `LockedFeature` with sensible defaults so a
// plugin page can degrade in one line:
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

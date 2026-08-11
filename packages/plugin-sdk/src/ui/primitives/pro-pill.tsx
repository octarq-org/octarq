import { ReactNode } from "react";
import { cn } from "../cn";

// ProPill is the small "Pro"/"Elite" tier badge shown next to gated features.
export function ProPill({ className, children }: { className?: string; children?: ReactNode }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full bg-gradient-to-r from-[color:color-mix(in_srgb,var(--accent-indigo)_25%,transparent)] to-[color:color-mix(in_srgb,var(--accent-violet)_25%,transparent)] px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-accent-fg ring-1 ring-inset ring-[color:color-mix(in_srgb,var(--primary)_30%,transparent)]",
        className,
      )}
    >
      {children ?? "Pro"}
    </span>
  );
}

// The tier → display-label map. Kept alongside ProPill so callers labelling a
// tier and rendering its pill share one source of truth.
export const TIER_LABEL: Record<"pro" | "elite", string> = { pro: "Pro", elite: "Elite" };

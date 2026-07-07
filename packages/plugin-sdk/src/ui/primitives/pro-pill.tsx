import { ReactNode } from "react";
import { cn } from "../cn";

// ProPill is the small "Pro"/"Elite" tier badge shown next to gated features.
export function ProPill({ className, children }: { className?: string; children?: ReactNode }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full bg-gradient-to-r from-indigo-500/25 to-violet-500/25 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-violet-200 ring-1 ring-inset ring-violet-400/30",
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

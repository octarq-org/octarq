import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";

// ProPill is the small "Pro"/"Elite" tier badge shown next to gated features.
export interface ProPillProps {
  className?: string;
  children?: ReactNode;
}

export const ProPill = forwardRef<HTMLSpanElement, ProPillProps>(({ className, children }, ref) => (
  <span
    ref={ref}
    className={cn(
      "inline-flex items-center gap-1 rounded-full border border-info-border bg-info-bg px-2 py-0.5 font-mono text-[10px] font-medium uppercase tracking-[0.08em] leading-none text-info-fg",
      className,
    )}
  >
    {children ?? "Pro"}
  </span>
));
ProPill.displayName = "ProPill";

// The tier → display-label map. Kept alongside ProPill so callers labelling a
// tier and rendering its pill share one source of truth.
export const TIER_LABEL: Record<"pro" | "elite", string> = { pro: "Pro", elite: "Elite" };

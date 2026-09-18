import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";

// GlassCard is the frosted surface every panel in octarq sits on. `strong` picks
// the higher-contrast `glass-strong` theme class (defined in the app's
// styles.css); the base variant uses `glass`.
export interface GlassCardProps {
  className?: string;
  children: ReactNode;
  strong?: boolean;
}

export const GlassCard = forwardRef<HTMLDivElement, GlassCardProps>(
  ({ className, children, strong }, ref) => (
    <div ref={ref} className={cn(strong ? "glass-strong" : "glass", "rounded-lg", className)}>
      {children}
    </div>
  ),
);
GlassCard.displayName = "GlassCard";

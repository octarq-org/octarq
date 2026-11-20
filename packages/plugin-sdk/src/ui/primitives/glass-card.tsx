import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";

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

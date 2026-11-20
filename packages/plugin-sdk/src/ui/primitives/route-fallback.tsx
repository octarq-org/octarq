import { forwardRef } from "react";
import { cn } from "../cn";

export interface RouteFallbackProps {
  className?: string;
}

export const RouteFallback = forwardRef<HTMLDivElement, RouteFallbackProps>(({ className }, ref) => (
  <div ref={ref} className={cn("grid h-64 place-items-center", className)} role="status" aria-live="polite">
    <div className="h-6 w-6 animate-spin rounded-full border-2 border-foreground/15 border-t-foreground/60" />
  </div>
));
RouteFallback.displayName = "RouteFallback";

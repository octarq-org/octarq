import { forwardRef } from "react";
import { cn } from "../cn";

// Fallback spinner while a lazily-loaded route chunk is fetched. Moved here from
// web/src/components/ui/RouteFallback.tsx so the shell's lazy boundaries and a
// plugin's own <Suspense> boundary render the same loading state — a plugin page
// must not white-screen while its chunk loads just because its Suspense boundary
// is closer than the shell's.
export interface RouteFallbackProps {
  className?: string;
}

export const RouteFallback = forwardRef<HTMLDivElement, RouteFallbackProps>(({ className }, ref) => (
  <div ref={ref} className={cn("grid h-64 place-items-center", className)} role="status" aria-live="polite">
    <div className="h-6 w-6 animate-spin rounded-full border-2 border-foreground/15 border-t-foreground/60" />
  </div>
));
RouteFallback.displayName = "RouteFallback";

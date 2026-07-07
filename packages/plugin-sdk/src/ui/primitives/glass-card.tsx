import { ReactNode } from "react";
import { cn } from "../cn";

// GlassCard is the frosted surface every panel in led sits on. `strong` picks
// the higher-contrast `glass-strong` theme class (defined in the app's
// styles.css); the base variant uses `glass`.
export function GlassCard({
  className,
  children,
  strong,
}: {
  className?: string;
  children: ReactNode;
  strong?: boolean;
}) {
  return (
    <div className={cn(strong ? "glass-strong" : "glass", "rounded-2xl", className)}>
      {children}
    </div>
  );
}

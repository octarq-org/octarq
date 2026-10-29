import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";
import { GlassCard } from "./glass-card";

export interface PanelProps {
  title: ReactNode;
  children: ReactNode;
  className?: string;
}

export const Panel = forwardRef<HTMLDivElement, PanelProps>(({ title, children, className }, ref) => (
  <GlassCard ref={ref} className={cn("p-6", className)}>
    <h3 className="eyebrow mb-4">{title}</h3>
    {children}
  </GlassCard>
));
Panel.displayName = "Panel";

import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";
import { GlassCard } from "./glass-card";

export interface PanelProps {
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  icon?: ReactNode;
  children?: ReactNode;
  className?: string;
  headingClassName?: string;
}

export const Panel = forwardRef<HTMLDivElement, PanelProps>(
  ({ title, description, action, icon, children, className, headingClassName }, ref) => (
    <GlassCard ref={ref} className={cn("p-5 space-y-4", className)}>
      <div className={cn("flex flex-wrap items-start justify-between gap-x-4 gap-y-2", headingClassName)}>
        <div className="min-w-0">
          <h2 className="flex items-center gap-2 text-base font-semibold tracking-tight text-foreground">
            {icon && <span className="flex shrink-0 items-center text-accent-fg">{icon}</span>}
            {title}
          </h2>
          {description && (
            <p className="mt-1 text-sm leading-relaxed text-muted-foreground">{description}</p>
          )}
        </div>
        {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
      </div>
      {children}
    </GlassCard>
  ),
);
Panel.displayName = "Panel";

import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";

// PageHeader is the standard title/description/action row atop every admin page.
export interface PageHeaderProps {
  title: ReactNode;
  description?: string;
  action?: ReactNode;
  className?: string;
}

export const PageHeader = forwardRef<HTMLDivElement, PageHeaderProps>(
  ({ title, description, action, className }, ref) => (
    <div ref={ref} className={cn("mb-6 flex flex-wrap items-start justify-between gap-4", className)}>
      <div>
        <h1 className="font-display text-2xl font-bold tracking-tight text-foreground">{title}</h1>
        {description && <p className="mt-1 text-sm text-foreground/55">{description}</p>}
      </div>
      {action}
    </div>
  ),
);
PageHeader.displayName = "PageHeader";

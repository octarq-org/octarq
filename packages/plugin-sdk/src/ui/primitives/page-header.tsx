import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";

export interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  eyebrow?: string;
  action?: ReactNode;
  className?: string;
}

export const PageHeader = forwardRef<HTMLDivElement, PageHeaderProps>(
  ({ title, description, eyebrow, action, className }, ref) => (
    <header
      ref={ref}
      data-slot="page-header"
      className={cn("flex flex-col gap-2", className)}
    >
      {eyebrow && (
        <span className="text-xs font-medium text-muted-foreground">{eyebrow}</span>
      )}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h1
          data-slot="page-title"
          className="text-2xl font-semibold tracking-tight text-foreground text-balance"
        >
          {title}
        </h1>
        {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
      </div>
      {description && (
        <p className="text-pretty text-sm leading-relaxed text-muted-foreground">
          {description}
        </p>
      )}
    </header>
  ),
);
PageHeader.displayName = "PageHeader";

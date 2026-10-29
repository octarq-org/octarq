import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";

// PageHeader is the standard title/description/action row atop every admin page.
// The eyebrow is the optional module label above the title ("Links", "Mail",
// "Workspace") — it gives every page the same three-step reading order:
// where am I, what is this, what can I do.
export interface PageHeaderProps {
  title: ReactNode;
  description?: string;
  eyebrow?: string;
  action?: ReactNode;
  className?: string;
}

export const PageHeader = forwardRef<HTMLDivElement, PageHeaderProps>(
  ({ title, description, eyebrow, action, className }, ref) => (
    <div ref={ref} className={cn("mb-8 flex flex-wrap items-end justify-between gap-x-6 gap-y-4", className)}>
      <div className="min-w-0">
        {eyebrow && <div className="eyebrow mb-2.5">{eyebrow}</div>}
        <h1 className="font-display text-[1.875rem] font-semibold leading-[1.12] tracking-[-0.03em] text-foreground text-balance">
          {title}
        </h1>
        {description && (
          <p className="mt-2 max-w-2xl text-[13.5px] leading-relaxed text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
    </div>
  ),
);
PageHeader.displayName = "PageHeader";

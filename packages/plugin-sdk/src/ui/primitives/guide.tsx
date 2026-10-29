import { forwardRef, useState, type ReactNode } from "react";
import { cn } from "../cn";

// Collapsible setup guidance panel. Moved here from the host app so a plugin
// package can ship the same affordance.
export interface GuideProps {
  title: string;
  children: ReactNode;
  open?: boolean;
  className?: string;
}

// Inline marks keep the package free of an icon dependency.
function BulbIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="h-3.5 w-3.5" aria-hidden="true">
      <path d="M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5" />
      <path d="M9 18h6" />
      <path d="M10 22h4" />
    </svg>
  );
}

function ChevronIcon({ open }: { open: boolean }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={cn(
        "h-4 w-4 transition-transform duration-[var(--dur-fast)] ease-[var(--ease-out)]",
        open && "rotate-180",
      )}
      aria-hidden="true"
    >
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
}

export const Guide = forwardRef<HTMLDivElement, GuideProps>(
  ({ title, children, open = false, className }, ref) => {
    const [show, setShow] = useState(open);
    return (
      <div ref={ref} className={cn("glass mb-4 overflow-hidden rounded-2xl", className)}>
        <button
          type="button"
          className="flex w-full cursor-pointer items-center justify-between gap-3 px-4 py-3.5 text-left text-[13px] font-medium text-foreground transition-colors duration-[var(--dur-fast)] hover:bg-surface-hover"
          onClick={() => setShow((s) => !s)}
          aria-expanded={show}
        >
          <span className="flex items-center gap-2.5">
            <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-accent-fg">
              <BulbIcon />
            </span>
            {title}
          </span>
          <span className="text-muted-foreground">
            <ChevronIcon open={show} />
          </span>
        </button>
        {show && (
          <div className="animate-expand space-y-2 border-t border-border px-4 py-4 text-[13px] leading-relaxed text-muted-foreground">
            {children}
          </div>
        )}
      </div>
    );
  },
);
Guide.displayName = "Guide";

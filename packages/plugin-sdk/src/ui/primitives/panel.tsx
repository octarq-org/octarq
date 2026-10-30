import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";
import { GlassCard } from "./glass-card";

// Panel is the standard titled section: a card with one heading row and its
// content. Every page section uses it, so section titles share one size,
// weight, and spacing instead of each page inventing an h2/h3/h4.
//
// The heading row reads the same three steps as PageHeader: what this section
// is (title), why it matters (description), what you can do here (action).
export interface PanelProps {
  title: ReactNode;
  description?: ReactNode;
  /** Rendered at the end of the heading row: a badge, a button, a switch. */
  action?: ReactNode;
  /** Rendered before the title, in the accent color. */
  icon?: ReactNode;
  /** Optional: a section may be all heading and action, like a settings switch. */
  children?: ReactNode;
  /** Extra classes on the card (padding, tone overrides). */
  className?: string;
  /** Extra classes on the heading row, for a description that needs width. */
  headingClassName?: string;
}

export const Panel = forwardRef<HTMLDivElement, PanelProps>(
  ({ title, description, action, icon, children, className, headingClassName }, ref) => (
    // The heading row and the content are siblings, so one `space-y-*` on the
    // card spaces both. cn() runs tailwind-merge, so a caller's own spacing
    // replaces the default instead of stacking a second utility.
    <GlassCard ref={ref} className={cn("p-6 space-y-4", className)}>
      <div className={cn("flex flex-wrap items-start justify-between gap-x-4 gap-y-2", headingClassName)}>
        <div className="min-w-0">
          <h2 className="flex items-center gap-2 font-display text-[15px] font-semibold tracking-[-0.01em] text-foreground">
            {icon && <span className="flex shrink-0 items-center text-accent-fg">{icon}</span>}
            {title}
          </h2>
          {description && (
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{description}</p>
          )}
        </div>
        {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
      </div>
      {children}
    </GlassCard>
  ),
);
Panel.displayName = "Panel";

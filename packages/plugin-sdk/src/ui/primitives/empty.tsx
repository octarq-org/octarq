import { forwardRef, type HTMLAttributes, type ReactNode } from "react";
import { cn } from "../cn";

// Empty is the centered "nothing here yet" placeholder card. The optional slots
// let a call site answer the three questions every empty state should: `reason`
// (why it's empty), `detail` (a concrete fact or path, machine values in mono),
// and `action` (the next-step control). `children` is the icon glyph only.
export interface EmptyProps extends Omit<HTMLAttributes<HTMLDivElement>, "children"> {
  /** The icon glyph. Optional: a short message needs no picture. */
  children?: ReactNode;
  reason?: ReactNode;
  detail?: ReactNode;
  action?: ReactNode;
}

export const Empty = forwardRef<HTMLDivElement, EmptyProps>(
  ({ children, reason, detail, action, className, ...rest }, ref) => (
    <div
      ref={ref}
      {...rest}
      className={cn(
        "glass flex flex-col items-center justify-center gap-3 rounded-2xl px-6 py-16 text-center",
        className,
      )}
    >
      {children && (
        <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-well text-accent-fg shadow-[var(--shadow-card)] ring-1 ring-border [&_svg]:h-5 [&_svg]:w-5">
          {children}
        </div>
      )}
      {reason && <p className="font-display text-[15px] text-foreground">{reason}</p>}
      {detail && (
        <p className="max-w-sm text-[12.5px] leading-relaxed text-muted-foreground">{detail}</p>
      )}
      {action && <div className="mt-2">{action}</div>}
    </div>
  ),
);
Empty.displayName = "Empty";

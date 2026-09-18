import { forwardRef, type ReactNode } from "react";
import { cn } from "../cn";

// Empty is the centered "nothing here yet" placeholder card. The optional slots
// let a call site answer the three questions every empty state should: `reason`
// (why it's empty), `detail` (a concrete fact or path, machine values in mono),
// and `action` (the next-step control). All slots are optional — the base
// `children` form keeps working unchanged.
export interface EmptyProps {
  children: ReactNode;
  reason?: ReactNode;
  detail?: ReactNode;
  action?: ReactNode;
  className?: string;
}

export const Empty = forwardRef<HTMLDivElement, EmptyProps>(
  ({ children, reason, detail, action, className }, ref) => (
    <div
      ref={ref}
      className={cn(
        "glass flex flex-col items-center justify-center gap-2 rounded-lg py-10 text-foreground/45",
        className,
      )}
    >
      {children}
      {reason && <p className="text-sm font-medium text-foreground/75">{reason}</p>}
      {detail && (
        <p className="max-w-md text-center text-[13px] leading-relaxed text-foreground/45">{detail}</p>
      )}
      {action && <div className="mt-1">{action}</div>}
    </div>
  ),
);
Empty.displayName = "Empty";

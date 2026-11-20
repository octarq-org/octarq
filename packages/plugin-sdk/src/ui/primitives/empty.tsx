import { forwardRef, type HTMLAttributes, type ReactNode } from "react";
import { cn } from "../cn";

export interface EmptyProps extends Omit<HTMLAttributes<HTMLDivElement>, "children"> {
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
        "flex flex-col items-center justify-center gap-3 rounded-lg border border-border bg-card px-6 py-12 text-center",
        className,
      )}
    >
      {children && (
        <div className="flex h-12 w-12 items-center justify-center rounded-lg border border-border bg-well text-muted-foreground [&_svg]:h-5 [&_svg]:w-5">
          {children}
        </div>
      )}
      {reason && <p className="text-sm font-medium text-foreground">{reason}</p>}
      {detail && (
        <p className="max-w-sm text-xs leading-relaxed text-muted-foreground">{detail}</p>
      )}
      {action && <div className="mt-1">{action}</div>}
    </div>
  ),
);
Empty.displayName = "Empty";

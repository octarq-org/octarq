import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../cn";

// Inline status surface. Every tone derives from a --{tone}-* token trio, so a
// white-label rebrand repaints it and no literal brand hue is ever needed.
// Moved here from web/src/components/ui/Alert.tsx: it was app-side only, which
// meant a plugin package could not render it at all and had to approximate one
// with a Badge.
export const alertVariants = cva(
  "relative flex w-full items-start gap-3 rounded-lg border p-4 text-[13px] leading-relaxed transition-colors",
  {
    variants: {
      variant: {
        info: "border-info-border bg-info-bg text-info-fg",
        success: "border-success-border bg-success-bg text-success-fg",
        warning: "border-warning-border bg-warning-bg text-warning-fg",
        danger: "border-danger-border bg-danger-bg text-danger-fg",
      },
    },
    defaultVariants: {
      variant: "info",
    },
  },
);

export interface AlertProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof alertVariants> {
  icon?: React.ReactNode;
  actions?: React.ReactNode;
  onDismiss?: () => void;
  align?: "start" | "center";
}

export const Alert = React.forwardRef<HTMLDivElement, AlertProps>(
  ({ className, variant, align = "start", icon, children, actions, onDismiss, ...props }, ref) => {
    return (
      <div
        ref={ref}
        role="alert"
        className={cn(
          alertVariants({ variant }),
          align === "center" && "items-center",
          className,
        )}
        {...props}
      >
        {icon && <div className={cn("shrink-0", align === "start" && "mt-0.5")}>{icon}</div>}
        <div className="flex-1 min-w-0">{children}</div>
        {(actions || onDismiss) && (
          <div className="shrink-0 flex items-center gap-2">
            {actions}
            {onDismiss && (
              <button
                type="button"
                onClick={onDismiss}
                className="opacity-70 hover:opacity-100 transition-opacity p-1 rounded-md cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-current"
                aria-label="Dismiss alert"
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>
        )}
      </div>
    );
  },
);
Alert.displayName = "Alert";

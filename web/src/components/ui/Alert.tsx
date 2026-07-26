import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib/utils";

export const alertVariants = cva(
  "relative w-full rounded-xl border p-4 text-sm font-medium transition-colors flex items-start gap-3",
  {
    variants: {
      variant: {
        info: "bg-info-bg text-info-fg border-info-border",
        success: "bg-success-bg text-success-fg border-success-border",
        warning: "bg-warning-bg text-warning-fg border-warning-border",
        danger: "bg-danger-bg text-danger-fg border-danger-border",
      },
    },
    defaultVariants: {
      variant: "info",
    },
  }
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
          className
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
                className="opacity-70 hover:opacity-100 transition-opacity p-1 rounded-md cursor-pointer"
                aria-label="Dismiss alert"
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>
        )}
      </div>
    );
  }
);
Alert.displayName = "Alert";

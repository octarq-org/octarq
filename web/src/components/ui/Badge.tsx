import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib/utils";

export const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors",
  {
    variants: {
      variant: {
        default: "bg-muted text-muted-foreground border-border",
        info: "bg-info-bg text-info-fg border-info-border",
        success: "bg-success-bg text-success-fg border-success-border",
        warning: "bg-warning-bg text-warning-fg border-warning-border",
        danger: "bg-danger-bg text-danger-fg border-danger-border",
        secondary: "bg-muted text-foreground border-transparent",
        outline: "text-foreground border-border",
        indigo: "bg-info-bg text-info-fg border-info-border",
        violet: "bg-info-bg text-info-fg border-info-border",
        green: "bg-success-bg text-success-fg border-success-border",
        amber: "bg-warning-bg text-warning-fg border-warning-border",
        red: "bg-danger-bg text-danger-fg border-danger-border",
        cyan: "bg-info-bg text-info-fg border-info-border",
        neutral: "bg-muted text-muted-foreground border-border",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

export type BadgeTone = "indigo" | "violet" | "green" | "amber" | "red" | "cyan" | "neutral" | "info" | "success" | "warning" | "danger";

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement> {
  variant?: VariantProps<typeof badgeVariants>["variant"];
  tone?: BadgeTone;
}

export const Badge = React.forwardRef<HTMLDivElement, BadgeProps>(
  ({ className, variant, tone, ...props }, ref) => {
    const resolvedVariant = (variant || tone || "default") as any;
    return (
      <div ref={ref} className={cn(badgeVariants({ variant: resolvedVariant }), className)} {...props} />
    );
  }
);
Badge.displayName = "Badge";

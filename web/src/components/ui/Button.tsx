import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib/utils";

export const buttonVariants = cva(
  "inline-flex items-center justify-center gap-1.5 rounded-[var(--radius)] font-medium transition-all focus-visible:outline-2 focus-visible:outline-(--ring) focus-visible:outline-offset-2 disabled:pointer-events-none disabled:opacity-50 cursor-pointer text-sm border-0",
  {
    variants: {
      variant: {
        primary: "brand-gradient text-white shadow-[inset_0_1px_0_rgba(255,255,255,0.22),0_8px_20px_-8px_rgba(99,102,241,0.45)] hover:brightness-108 active:translate-y-[1px]",
        secondary: "bg-muted text-foreground hover:bg-surface-hover active:translate-y-[1px] border border-border",
        subtle: "bg-muted text-foreground hover:bg-surface-hover active:translate-y-[1px] border border-transparent",
        ghost: "bg-transparent text-muted-foreground hover:bg-surface-hover hover:text-foreground active:translate-y-[1px]",
        danger: "bg-transparent text-danger-fg hover:bg-danger-bg/50 active:translate-y-[1px]",
        outline: "border border-border bg-card text-foreground hover:bg-surface-hover hover:border-border-strong active:translate-y-[1px]",
      },
      size: {
        sm: "h-8 px-3 text-xs",
        md: "h-9 px-3.5 text-sm",
        lg: "h-10 px-4 text-base",
      },
    },
    defaultVariants: {
      variant: "primary",
      size: "md",
    },
  }
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => {
    return (
      <button
        ref={ref}
        className={cn(buttonVariants({ variant, size }), className)}
        {...props}
      />
    );
  }
);
Button.displayName = "Button";

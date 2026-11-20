import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../cn";

export const buttonVariants = cva(
  "inline-flex cursor-pointer select-none items-center justify-center gap-2 rounded-[var(--radius)] border-0 font-medium tracking-[-0.01em] outline-none transition-colors duration-100 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        primary:
          "bg-primary-solid text-primary-foreground hover:bg-primary-solid-hover",
        secondary:
          "border border-border bg-card text-foreground hover:border-border-strong hover:bg-surface-hover",
        subtle: "bg-muted text-foreground hover:bg-surface-hover",
        ghost:
          "bg-transparent text-muted-foreground hover:bg-surface-hover hover:text-foreground",
        outline:
          "border border-border bg-transparent text-foreground hover:border-border-strong hover:bg-surface-hover",
        danger: "bg-transparent text-danger-fg hover:bg-danger-bg",
        "danger-solid":
          "bg-danger-solid text-danger-solid-fg hover:brightness-110",
      },
      size: {
        sm: "h-8 gap-1.5 px-3 text-xs",
        md: "h-10 px-4 text-[13px]",
        lg: "h-11 px-5 text-sm",
      },
    },
    defaultVariants: { variant: "primary", size: "md" },
  },
);

export type ButtonVariant = NonNullable<VariantProps<typeof buttonVariants>["variant"]>;
export type ButtonSize = NonNullable<VariantProps<typeof buttonVariants>["size"]>;

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => (
    <button ref={ref} className={cn(buttonVariants({ variant, size }), className)} {...props} />
  ),
);
Button.displayName = "Button";

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../cn";

// The single Button for the whole product — core pages AND plugin packages.
//
// One definition, one variant vocabulary. It used to exist twice: this package
// carried a gradient (indigo→violet) primary, while the app carried a flat one
// in web/src/components/ui/Button.tsx that the app barrel re-exported in its
// place. Core pages rendered the flat button, every plugin rendered the
// gradient one, and `size`/`secondary` existed on only one of them.
//
// The FLAT fill won: a gradient cannot follow a white-label rebrand, which
// overrides only the --primary seed (brand.tsx applyAccents) — a hardcoded
// gradient end stays indigo on a rebranded instance, exactly the failure the
// token pipeline exists to prevent. Flat also means the primary action and the
// accent chrome share one source of truth.
//
// Radius stays `rounded-xl`, which the host's @theme pins to --radius (4px) —
// identical to what the app-local button rendered, and still valid for a
// consumer that ships its own theme (the Pro portal) where `var(--radius)` is
// not defined at all.
export const buttonVariants = cva(
  "inline-flex items-center justify-center gap-1.5 rounded-xl border-0 text-sm font-medium transition-all focus-visible:outline-2 focus-visible:outline-(--ring) focus-visible:outline-offset-2 disabled:pointer-events-none disabled:opacity-50 cursor-pointer",
  {
    variants: {
      variant: {
        primary: "bg-primary text-primary-foreground hover:bg-primary-hover active:translate-y-[1px]",
        secondary: "bg-muted text-foreground hover:bg-surface-hover border border-border active:translate-y-[1px]",
        subtle: "bg-muted text-foreground hover:bg-surface-hover border border-transparent active:translate-y-[1px]",
        ghost: "bg-transparent text-muted-foreground hover:bg-surface-hover hover:text-foreground active:translate-y-[1px]",
        outline: "border border-border bg-card text-foreground hover:bg-surface-hover hover:border-border-strong active:translate-y-[1px]",
        // Token-driven, not a literal rose: --danger-* already carries the hue
        // in both themes, so no `ui-color-ok` exemption is needed.
        danger: "bg-transparent text-danger-fg hover:bg-danger-bg/50 active:translate-y-[1px]",
      },
      size: {
        sm: "h-8 px-3 text-xs",
        md: "h-9 px-3.5 text-sm",
        lg: "h-10 px-4 text-base",
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

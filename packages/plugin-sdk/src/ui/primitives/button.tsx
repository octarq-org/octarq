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
// Geometry stays token-driven: the control radius is --radius (10px on the
// host), so the button matches every field and chip next to it, and still works
// for a consumer that ships its own theme (the Pro portal).
export const buttonVariants = cva(
  "inline-flex cursor-pointer select-none items-center justify-center gap-2 rounded-[var(--radius)] border-0 font-medium tracking-[-0.01em] outline-none transition-[background-color,color,border-color,box-shadow,transform] duration-[var(--dur-fast)] ease-[var(--ease-out)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--ring) active:scale-[0.985] disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        primary:
          "bg-primary-solid text-primary-foreground shadow-[var(--shadow-accent)] hover:-translate-y-px hover:bg-primary-solid-hover",
        secondary:
          "border border-border bg-card text-foreground shadow-[0_1px_1px_rgb(0_0_0/0.03)] hover:border-border-strong hover:bg-surface-hover",
        subtle: "bg-muted text-foreground hover:bg-surface-hover",
        ghost:
          "bg-transparent text-muted-foreground hover:bg-surface-hover hover:text-foreground",
        outline:
          "border border-border bg-transparent text-foreground hover:border-border-strong hover:bg-surface-hover",
        // Token-driven, not a literal rose: --danger-* already carries the hue
        // in both themes, so no `ui-color-ok` exemption is needed.
        danger: "bg-transparent text-danger-fg hover:bg-danger-bg",
        // The filled destructive action for confirm dialogs. Its pair of
        // *-solid tokens keeps the foreground readable in both themes, so this
        // variant needs no per-call-site override.
        "danger-solid":
          "bg-danger-solid text-danger-solid-fg shadow-[0_8px_20px_-12px_var(--danger-solid)] hover:brightness-110",
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

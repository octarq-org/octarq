import { ReactNode } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../cn";

export const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 rounded-xl px-3.5 py-2 text-sm font-medium transition-[color,background-color,border-color,box-shadow,filter,transform] duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--ring)] focus-visible:ring-offset-0 active:translate-y-px disabled:cursor-not-allowed disabled:opacity-50 disabled:active:translate-y-0",
  {
    variants: {
      variant: {
        // Primary carries the brand gradient (indigo→violet, same axis as the
        // TopBar mark) with an inset top highlight so it reads as a lit surface.
        primary:
          "bg-[image:var(--gradient-primary)] text-[color:var(--primary-foreground)] " +
          "shadow-[inset_0_1px_0_rgba(255,255,255,0.22),0_8px_30px_-8px_color-mix(in_srgb,var(--primary)_55%,transparent)] " +
          "hover:brightness-110 disabled:hover:brightness-100",
        ghost:   "text-foreground/65 hover:text-foreground hover:bg-foreground/[0.06]",
        outline: "border border-foreground/10 text-foreground/80 hover:bg-foreground/[0.06] hover:border-foreground/20",
        subtle:  "bg-foreground/5 text-foreground/80 hover:bg-foreground/10 hover:text-foreground",
        danger:  "text-danger-fg/90 hover:bg-rose-500/10 hover:text-danger-fg", /* ui-color-ok */
      },
    },
    defaultVariants: { variant: "primary" },
  },
);

export type ButtonVariant = NonNullable<VariantProps<typeof buttonVariants>["variant"]>;

export function Button({
  children,
  variant = "primary",
  className,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant; children?: ReactNode }) {
  return (
    <button className={cn(buttonVariants({ variant }), className)} {...props}>
      {children}
    </button>
  );
}

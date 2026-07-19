import { ReactNode } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../cn";

export const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 rounded-xl px-3.5 py-2 text-sm font-medium transition-[color,background-color,border-color,box-shadow,filter,transform] duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400/60 active:translate-y-px disabled:cursor-not-allowed disabled:opacity-50 disabled:active:translate-y-0",
  {
    variants: {
      variant: {
        // Primary carries the brand gradient (indigo→violet, same axis as the
        // TopBar mark) with an inset top highlight so it reads as a lit surface.
        primary:
          "bg-gradient-to-br from-indigo-500 via-[#7c5cf6] to-violet-500 text-white " +
          "shadow-[inset_0_1px_0_rgba(255,255,255,0.22),0_8px_30px_-8px_rgba(99,102,241,0.55)] " +
          "hover:brightness-110 disabled:hover:brightness-100",
        ghost:   "text-white/65 hover:text-white hover:bg-white/[0.06]",
        outline: "border border-white/10 text-white/80 hover:bg-white/[0.06] hover:border-white/20",
        subtle:  "bg-white/5 text-white/80 hover:bg-white/10 hover:text-white",
        danger:  "text-rose-300/90 hover:bg-rose-500/10 hover:text-rose-300",
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

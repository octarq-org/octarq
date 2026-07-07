import { ReactNode } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../cn";

export const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 rounded-xl px-3.5 py-2 text-sm font-medium transition-colors duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400/60 disabled:cursor-not-allowed disabled:opacity-50",
  {
    variants: {
      variant: {
        primary: "bg-indigo-500 text-white hover:bg-indigo-400 shadow-[0_8px_30px_-8px_rgba(99,102,241,0.6)]",
        ghost:   "text-white/65 hover:text-white hover:bg-white/5",
        outline: "border border-white/10 text-white/80 hover:bg-white/5 hover:border-white/20",
        subtle:  "bg-white/5 text-white/80 hover:bg-white/10",
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

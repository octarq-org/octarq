import { ReactNode } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../cn";

// cva variants — the shadcn pattern: a base class string plus a `tone` axis,
// combined with the caller's className through cn().
export const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset",
  {
    variants: {
      tone: {
        indigo:  "bg-indigo-500/15 text-indigo-300 ring-indigo-400/20",
        violet:  "bg-violet-500/15 text-violet-300 ring-violet-400/20",
        green:   "bg-emerald-500/15 text-emerald-300 ring-emerald-400/20",
        amber:   "bg-amber-500/15  text-amber-300  ring-amber-400/20",
        red:     "bg-rose-500/15   text-rose-300   ring-rose-400/20",
        cyan:    "bg-cyan-500/15   text-cyan-300   ring-cyan-400/20",
        neutral: "bg-white/[0.08]  text-white/70   ring-white/10",
      },
    },
    defaultVariants: { tone: "neutral" },
  },
);

export type BadgeTone = NonNullable<VariantProps<typeof badgeVariants>["tone"]>;

export function Badge({
  children,
  tone = "neutral",
  className,
}: {
  children: ReactNode;
  tone?: BadgeTone;
  className?: string;
}) {
  return <span className={cn(badgeVariants({ tone }), className)}>{children}</span>;
}

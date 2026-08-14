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
        indigo:  "bg-indigo-500/15 text-accent-fg ring-indigo-400/20", /* ui-color-ok */
        violet:  "bg-violet-500/15 text-accent-fg ring-violet-400/20", /* ui-color-ok */
        green:   "bg-emerald-500/15 text-success-fg ring-emerald-400/20", /* ui-color-ok */
        amber:   "bg-amber-500/15  text-warning-fg  ring-amber-400/20", /* ui-color-ok */
        red:     "bg-rose-500/15   text-danger-fg   ring-rose-400/20", /* ui-color-ok */
        cyan:    "bg-cyan-500/15   text-cyan-300   ring-cyan-400/20",
        neutral: "bg-foreground/[0.08]  text-foreground/70   ring-foreground/10",
      },
    },
    defaultVariants: { tone: "neutral" },
  },
);

export type BadgeTone = NonNullable<VariantProps<typeof badgeVariants>["tone"]>;

// Optional `shape` prefix: a glyph channel so status reads without color
// (grayscale / color-blind); the text label is the third channel.
export type BadgeShape = "square" | "square-outline" | "dot" | "dash";

const SHAPE_GLYPH: Record<BadgeShape, string> = {
  square: "■",
  "square-outline": "□",
  dot: "●",
  dash: "—",
};

export function Badge({
  children,
  tone = "neutral",
  shape,
  className,
}: {
  children: ReactNode;
  tone?: BadgeTone;
  shape?: BadgeShape;
  className?: string;
}) {
  return (
    <span className={cn(badgeVariants({ tone }), className)}>
      {shape && (
        <span aria-hidden className="text-[0.9em] leading-none">
          {SHAPE_GLYPH[shape]}
        </span>
      )}
      {children}
    </span>
  );
}

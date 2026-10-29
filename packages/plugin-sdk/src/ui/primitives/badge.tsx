import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../cn";

// cva variants — the shadcn pattern: a base class string plus a `tone` axis,
// combined with the caller's className through cn().
// `variant` and `tone` are accepted as synonyms for this axis (see BadgeProps):
// both spellings exist across call sites and must keep working.
export const badgeVariants = cva(
  "inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-2 py-0.5 text-[11px] font-semibold leading-none tracking-[0.01em] transition-colors",
  {
    variants: {
      tone: {
        default: "border-border bg-muted/70 text-muted-foreground",
        info: "border-info-border bg-info-bg text-info-fg",
        success: "border-success-border bg-success-bg text-success-fg",
        warning: "border-warning-border bg-warning-bg text-warning-fg",
        danger: "border-danger-border bg-danger-bg text-danger-fg",
        secondary: "border-transparent bg-muted text-foreground",
        outline: "border-border bg-transparent text-foreground",
        indigo: "border-info-border bg-info-bg text-info-fg",
        violet: "border-info-border bg-info-bg text-info-fg",
        green: "border-success-border bg-success-bg text-success-fg",
        amber: "border-warning-border bg-warning-bg text-warning-fg",
        red: "border-danger-border bg-danger-bg text-danger-fg",
        cyan: "border-info-border bg-info-bg text-info-fg",
        neutral: "border-border bg-muted/70 text-muted-foreground",
      },
    },
    defaultVariants: { tone: "default" },
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

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeTone;
  tone?: BadgeTone;
  shape?: BadgeShape;
}

export const Badge = React.forwardRef<HTMLSpanElement, BadgeProps>(
  ({ className, variant, tone, shape, children, ...rest }, ref) => {
    const resolvedTone = variant || tone || "default";
    return (
      <span ref={ref} className={cn(badgeVariants({ tone: resolvedTone }), className)} {...rest}>
        {shape && (
          <span aria-hidden className="text-[0.9em] leading-none">
            {SHAPE_GLYPH[shape]}
          </span>
        )}
        {children}
      </span>
    );
  },
);
Badge.displayName = "Badge";

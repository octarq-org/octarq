import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../cn";

// cva variants — the shadcn pattern: a base class string plus a `tone` axis,
// combined with the caller's className through cn().
// `variant` and `tone` are accepted as synonyms for this axis (see BadgeProps):
// both spellings exist across call sites and must keep working.
export const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors",
  {
    variants: {
      tone: {
        default: "bg-muted text-muted-foreground border-border",
        info: "bg-info-bg text-info-fg border-info-border",
        success: "bg-success-bg text-success-fg border-success-border",
        warning: "bg-warning-bg text-warning-fg border-warning-border",
        danger: "bg-danger-bg text-danger-fg border-danger-border",
        secondary: "bg-muted text-foreground border-transparent",
        outline: "text-foreground border-border",
        indigo: "bg-info-bg text-info-fg border-info-border",
        violet: "bg-info-bg text-info-fg border-info-border",
        green: "bg-success-bg text-success-fg border-success-border",
        amber: "bg-warning-bg text-warning-fg border-warning-border",
        red: "bg-danger-bg text-danger-fg border-danger-border",
        cyan: "bg-info-bg text-info-fg border-info-border",
        neutral: "bg-muted text-muted-foreground border-border",
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

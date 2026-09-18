import { forwardRef, type HTMLAttributes } from "react";
import { cn } from "../cn";

// Skeleton is a pulsing placeholder block for loading states. Size it with
// className (e.g. `h-4 w-32`); defaults to a full-width line.
export type SkeletonProps = HTMLAttributes<HTMLDivElement>;

export const Skeleton = forwardRef<HTMLDivElement, SkeletonProps>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    aria-hidden
    className={cn("h-4 w-full animate-pulse rounded-md bg-foreground/[0.08]", className)}
    {...props}
  />
));
Skeleton.displayName = "Skeleton";

import { forwardRef, type HTMLAttributes } from "react";
import { cn } from "../cn";

// Skeleton is a pulsing placeholder block for loading states. Size it with
// className (e.g. `h-4 w-32`); defaults to a full-width line.
export type SkeletonProps = HTMLAttributes<HTMLDivElement>;

export const Skeleton = forwardRef<HTMLDivElement, SkeletonProps>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    aria-hidden
    className={cn("skeleton h-4 w-full rounded-md", className)}
    {...props}
  />
));
Skeleton.displayName = "Skeleton";

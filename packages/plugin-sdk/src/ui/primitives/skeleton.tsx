import { HTMLAttributes } from "react";
import { cn } from "../cn";

// Skeleton is a pulsing placeholder block for loading states. Size it with
// className (e.g. `h-4 w-32`); defaults to a full-width line.
export function Skeleton({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      aria-hidden
      className={cn("h-4 w-full animate-pulse rounded-md bg-white/[0.08]", className)}
      {...props}
    />
  );
}

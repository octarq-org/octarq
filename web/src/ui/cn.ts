import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// cn is the shadcn-standard class combiner: clsx resolves conditional/array
// class inputs, then tailwind-merge dedupes conflicting Tailwind utilities so a
// caller's `className` can override a component's defaults. Every primitive in
// this folder composes classes through it.
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}

import { forwardRef, type TextareaHTMLAttributes } from "react";
import { cn } from "../cn";
import { fieldClass } from "./input";

export type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

// Textarea is a native <textarea> carrying the same glass field styling as
// Input. Base UI has no dedicated textarea primitive; a native element with the
// shared styling is the idiomatic choice.
export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, rows = 3, ...props }, ref) => (
    <textarea ref={ref} rows={rows} className={cn(fieldClass, "resize-y", className)} {...props} />
  ),
);
Textarea.displayName = "Textarea";

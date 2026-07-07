import { TextareaHTMLAttributes } from "react";
import { cn } from "../cn";
import { fieldClass } from "./input";

// Textarea is a native <textarea> carrying the same glass field styling as
// Input. Base UI has no dedicated textarea primitive; a native element with the
// shared styling is the idiomatic choice.
export function Textarea({
  className,
  rows = 3,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea rows={rows} className={cn(fieldClass, "resize-y", className)} {...props} />;
}

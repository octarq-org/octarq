import { forwardRef, type Ref, type TextareaHTMLAttributes } from "react";
import { Field as BaseField } from "@base-ui/react/field";
import { cn } from "../cn";
import { fieldClass } from "./input";

export type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, rows = 3, disabled, ...props }, ref) => (
    <BaseField.Control
      ref={ref as Ref<HTMLElement>}
      disabled={disabled}
      className={cn(fieldClass, "resize-y", className)}
      render={<textarea rows={rows} disabled={disabled} {...props} />}
    />
  ),
);
Textarea.displayName = "Textarea";

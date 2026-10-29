import { forwardRef, type Ref, type TextareaHTMLAttributes } from "react";
import { Field as BaseField } from "@base-ui/react/field";
import { cn } from "../cn";
import { fieldClass } from "./input";

export type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

// Textarea is the native <textarea> counterpart of Input: a Field.Control with
// a textarea render target, so the label and hint of an enclosing Field reach
// it the same way they reach Input (for/id + aria-describedby). A bare
// <textarea> would leave the control unnamed inside a Field. Rendered outside a
// Field it falls back to Base UI's default (empty) field context.
export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, rows = 3, disabled, ...props }, ref) => (
    <BaseField.Control
      // Field.Control's own props are typed for <input>; the render target is
      // where the textarea types apply.
      ref={ref as Ref<HTMLElement>}
      // `disabled` also goes to Field.Control itself: the control registers its
      // own validity with the field, and the DOM attribute alone would leave a
      // disabled textarea participating as an enabled control.
      disabled={disabled}
      className={cn(fieldClass, "resize-y", className)}
      render={<textarea rows={rows} disabled={disabled} {...props} />}
    />
  ),
);
Textarea.displayName = "Textarea";

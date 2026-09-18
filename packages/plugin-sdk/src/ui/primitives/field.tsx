import { forwardRef, type ReactNode } from "react";
import { Field as BaseField } from "@base-ui/react/field";
import { cn } from "../cn";

// Field is the label/control/hint stack used by every form row, backed by Base
// UI's Field so the label and hint auto-associate (for/id + aria-describedby)
// with a Base UI control inside — Input, Textarea, and Select from this package
// all participate, so their pages get that wiring for free. `label` uses the
// app's `.label` theme class (defined in styles.css).
//
// The ref lands on the field container that wraps the label, control and hint.
export interface FieldProps {
  label: string;
  children: ReactNode;
  hint?: string;
  className?: string;
}

export const Field = forwardRef<HTMLDivElement, FieldProps>(
  ({ label, children, hint, className }, ref) => (
    <BaseField.Root ref={ref} className={cn("mb-3", className)}>
      <BaseField.Label className="label">{label}</BaseField.Label>
      {children}
      {hint && (
        <BaseField.Description className="mt-1 text-xs text-foreground/40">
          {hint}
        </BaseField.Description>
      )}
    </BaseField.Root>
  ),
);
Field.displayName = "Field";

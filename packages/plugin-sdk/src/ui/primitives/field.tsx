import { ReactNode } from "react";
import { Field as BaseField } from "@base-ui/react/field";

// Field is the label/control/hint stack used by every form row, backed by Base
// UI's Field so the label and hint auto-associate (for/id + aria-describedby)
// with a Base UI control inside — Input, Textarea, and Select from this package
// all participate, so their pages get that wiring for free. `label` uses the
// app's `.label` theme class (defined in styles.css).
export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <BaseField.Root className="mb-3">
      <BaseField.Label className="label">{label}</BaseField.Label>
      {children}
      {hint && (
        <BaseField.Description className="mt-1 text-xs text-white/40">
          {hint}
        </BaseField.Description>
      )}
    </BaseField.Root>
  );
}

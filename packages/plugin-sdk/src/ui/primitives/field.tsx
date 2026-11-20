import { forwardRef, type ReactNode } from "react";
import { Field as BaseField } from "@base-ui/react/field";
import { cn } from "../cn";

export interface FieldProps {
  label: string;
  children: ReactNode;
  hint?: string;
  className?: string;
}

export const Field = forwardRef<HTMLDivElement, FieldProps>(
  ({ label, children, hint, className }, ref) => (
    <BaseField.Root ref={ref} className={cn("mb-4", className)}>
      <BaseField.Label className="label">{label}</BaseField.Label>
      {children}
      {hint && (
        <BaseField.Description className="mt-1.5 text-[12px] leading-relaxed text-muted-foreground">
          {hint}
        </BaseField.Description>
      )}
    </BaseField.Root>
  ),
);
Field.displayName = "Field";

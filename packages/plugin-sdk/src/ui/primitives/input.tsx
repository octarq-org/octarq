import { forwardRef, type InputHTMLAttributes } from "react";
import { Input as BaseInput } from "@base-ui/react/input";
import { cn } from "../cn";

// The shared glass field styling, expressed as Tailwind utilities so the
// component is self-contained (mirrors the app's `.input` class without
// depending on it).
export const fieldClass =
  "w-full rounded-[var(--radius)] border border-input bg-card px-3.5 py-2.5 text-[13.5px] leading-5 text-foreground shadow-[inset_0_1px_1px_rgb(0_0_0/0.02)] outline-none transition-[border-color,box-shadow,background-color] duration-[var(--dur-fast)] ease-[var(--ease-out)] " +
  "placeholder:text-muted-foreground hover:border-border-strong focus-visible:border-accent-border focus-visible:shadow-[0_0_0_3px_var(--accent-soft)] focus:outline-none " +
  "disabled:cursor-not-allowed disabled:opacity-50";

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

// Input wraps Base UI's Input primitive (which auto-integrates with Base UI
// Field for validation/aria when nested in one) and carries octarq's glass theme.
// Accepts all native <input> attributes.
export const Input = forwardRef<HTMLInputElement, InputProps>(({ className, ...props }, ref) => (
  <BaseInput ref={ref} className={cn(fieldClass, "h-10 py-0", className)} {...props} />
));
Input.displayName = "Input";

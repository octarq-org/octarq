import { InputHTMLAttributes } from "react";
import { Input as BaseInput } from "@base-ui/react/input";
import { cn } from "../cn";

// The shared glass field styling, expressed as Tailwind utilities so the
// component is self-contained (mirrors the app's `.input` class without
// depending on it).
export const fieldClass =
  "w-full rounded-xl border border-white/10 bg-white/[0.04] px-3 py-2 text-sm text-white outline-none transition-all " +
  "placeholder:text-white/30 focus:border-indigo-400/40 focus:bg-white/[0.06] focus:shadow-[0_0_0_1px_rgba(99,102,241,0.30)] " +
  "disabled:cursor-not-allowed disabled:opacity-50";

// Input wraps Base UI's Input primitive (which auto-integrates with Base UI
// Field for validation/aria when nested in one) and carries octarq's glass theme.
// Accepts all native <input> attributes.
export function Input({
  className,
  ...props
}: InputHTMLAttributes<HTMLInputElement>) {
  return <BaseInput className={cn(fieldClass, className)} {...props} />;
}

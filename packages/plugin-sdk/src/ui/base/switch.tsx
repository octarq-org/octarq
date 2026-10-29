import { forwardRef } from "react";
import { Switch as BaseSwitch } from "@base-ui/react/switch";
import { cn } from "../cn";

// Switch is a shadcn-style wrapper over Base UI's accessible Switch primitive
// (role="switch", keyboard-toggleable, focus-visible ring) carrying octarq's glass
// theme. Base UI renders this as a <span>, so `inline-flex` is load-bearing:
// without it the span stays inline, ignores h-5/w-9, and the track never paints.
//
// The ref lands on the switch control itself — the element that takes focus and
// keyboard input.
export interface SwitchProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
  "aria-label"?: string;
}

export const Switch = forwardRef<HTMLSpanElement, SwitchProps>(
  ({ checked, onCheckedChange, disabled, className, "aria-label": ariaLabel }, ref) => (
    <BaseSwitch.Root
      ref={ref}
      checked={checked}
      onCheckedChange={(v) => onCheckedChange(v)}
      disabled={disabled}
      aria-label={ariaLabel}
      className={cn(
        "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full outline-none transition-colors duration-[var(--dur-base)] ease-[var(--ease-out)]",
        "focus-visible:ring-2 focus-visible:ring-ring/60 disabled:cursor-not-allowed disabled:opacity-50",
        "bg-border data-[checked]:bg-primary",
        className,
      )}
    >
      <BaseSwitch.Thumb
        className={cn(
          "absolute left-0.5 top-0.5 h-5 w-5 rounded-full bg-white shadow-[0_1px_2px_rgb(0_0_0/0.25)] transition-[left,transform] duration-[var(--dur-base)] ease-[var(--ease-out)]",
          "data-[checked]:left-[1.375rem]",
        )}
      />
    </BaseSwitch.Root>
  ),
);
Switch.displayName = "Switch";

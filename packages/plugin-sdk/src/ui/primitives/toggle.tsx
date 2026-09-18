import { forwardRef } from "react";
import { Switch } from "../base/switch";

// Toggle keeps its `{ on, onChange }` API but is the accessible Base UI Switch
// (role="switch", keyboard-operable, focus-visible ring) instead of a bare
// <button>. The ref passes straight through to the switch control.
export interface ToggleProps {
  on: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
  className?: string;
}

export const Toggle = forwardRef<HTMLButtonElement, ToggleProps>(
  ({ on, onChange, disabled, className }, ref) => (
    <Switch ref={ref} checked={on} onCheckedChange={onChange} disabled={disabled} className={className} />
  ),
);
Toggle.displayName = "Toggle";

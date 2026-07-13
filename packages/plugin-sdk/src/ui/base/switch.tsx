import { Switch as BaseSwitch } from "@base-ui/react/switch";
import { cn } from "../cn";

// Switch is a shadcn-style wrapper over Base UI's accessible Switch primitive
// (role="switch", keyboard-toggleable, focus-visible ring) carrying octarq's glass
// theme. The higher-level `Toggle` in ../primitives adapts it to the app's
// `{ on, onChange }` API; plugin authors can use either.
export function Switch({
  checked,
  onCheckedChange,
  disabled,
  className,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
}) {
  return (
    <BaseSwitch.Root
      checked={checked}
      onCheckedChange={(v) => onCheckedChange(v)}
      disabled={disabled}
      className={cn(
        "relative h-5 w-9 shrink-0 rounded-full outline-none transition-colors duration-300",
        "focus-visible:ring-2 focus-visible:ring-indigo-400/60 disabled:cursor-not-allowed disabled:opacity-50",
        "bg-white/15 data-[checked]:bg-indigo-500",
        className,
      )}
    >
      <BaseSwitch.Thumb
        className={cn(
          "absolute top-0.5 h-4 w-4 rounded-full bg-white shadow-sm transition-all duration-300",
          "left-0.5 scale-90 opacity-70 data-[checked]:left-4 data-[checked]:scale-110 data-[checked]:opacity-100",
        )}
      />
    </BaseSwitch.Root>
  );
}

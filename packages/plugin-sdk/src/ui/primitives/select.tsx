import { ReactNode } from "react";
import { Select as BaseSelect } from "@base-ui/react/select";
import { cn } from "../cn";

export interface SelectOption {
  value: string;
  label: ReactNode;
  disabled?: boolean;
}

// Select wraps Base UI's Select primitive — an accessible, keyboard-navigable
// listbox with a portalled, positioned popup — into a compact `{ value, onValueChange,
// options }` API carrying octarq's glass theme. For richer composition use Base
// UI's Select parts directly.
export function Select({
  value,
  onValueChange,
  options,
  placeholder,
  disabled,
  className,
  id,
  name,
}: {
  value?: string;
  onValueChange?: (value: string) => void;
  options: SelectOption[];
  placeholder?: ReactNode;
  disabled?: boolean;
  className?: string;
  id?: string;
  name?: string;
}) {
  return (
    <BaseSelect.Root
      items={options}
      value={value ?? null}
      onValueChange={(v) => { if (v != null) onValueChange?.(v); }}
      disabled={disabled}
      id={id}
      name={name}
    >
      <BaseSelect.Trigger
        className={cn(
          "flex w-full items-center justify-between gap-2 rounded-xl border border-input bg-card px-3 py-2 text-sm text-foreground outline-none transition-all",
          "hover:border-border-strong focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30",
          "data-[popup-open]:border-accent-border disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
      >
        <BaseSelect.Value placeholder={placeholder} />
        <BaseSelect.Icon className="text-foreground/40">▾</BaseSelect.Icon>
      </BaseSelect.Trigger>
      <BaseSelect.Portal>
        <BaseSelect.Positioner sideOffset={6} className="z-50 outline-none" alignItemWithTrigger={false}>
          <BaseSelect.Popup
            className={cn(
              "glass-strong max-h-[min(24rem,var(--available-height))] min-w-[var(--anchor-width)] overflow-y-auto rounded-xl p-1 outline-none",
              "origin-[var(--transform-origin)]",
            )}
          >
            {options.map((opt) => (
              <BaseSelect.Item
                key={opt.value}
                value={opt.value}
                disabled={opt.disabled}
                className={cn(
                  "flex cursor-pointer items-center justify-between gap-3 rounded-lg px-2.5 py-1.5 text-sm text-foreground/80 outline-none",
                  "data-[highlighted]:bg-foreground/10 data-[highlighted]:text-foreground data-[disabled]:cursor-not-allowed data-[disabled]:opacity-40",
                )}
              >
                <BaseSelect.ItemText>{opt.label}</BaseSelect.ItemText>
                <BaseSelect.ItemIndicator className="text-accent-fg">✓</BaseSelect.ItemIndicator>
              </BaseSelect.Item>
            ))}
          </BaseSelect.Popup>
        </BaseSelect.Positioner>
      </BaseSelect.Portal>
    </BaseSelect.Root>
  );
}

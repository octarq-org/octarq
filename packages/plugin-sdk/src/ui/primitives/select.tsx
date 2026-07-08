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
          "flex w-full items-center justify-between gap-2 rounded-xl border border-white/10 bg-white/[0.04] px-3 py-2 text-sm text-white outline-none transition-all",
          "hover:bg-white/[0.06] focus-visible:border-indigo-400/40 focus-visible:shadow-[0_0_0_1px_rgba(99,102,241,0.30)]",
          "data-[popup-open]:border-indigo-400/40 disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
      >
        <BaseSelect.Value placeholder={placeholder} />
        <BaseSelect.Icon className="text-white/40">▾</BaseSelect.Icon>
      </BaseSelect.Trigger>
      <BaseSelect.Portal>
        <BaseSelect.Positioner sideOffset={6} className="z-50 outline-none" alignItemWithTrigger={false}>
          <BaseSelect.Popup
            className={cn(
              "glass-strong max-h-[min(24rem,var(--available-height))] min-w-[var(--anchor-width)] overflow-y-auto rounded-xl p-1 outline-none",
              "origin-[var(--transform-origin)] shadow-[0_16px_48px_-12px_rgba(0,0,0,0.6)]",
            )}
          >
            {options.map((opt) => (
              <BaseSelect.Item
                key={opt.value}
                value={opt.value}
                disabled={opt.disabled}
                className={cn(
                  "flex cursor-pointer items-center justify-between gap-3 rounded-lg px-2.5 py-1.5 text-sm text-white/80 outline-none",
                  "data-[highlighted]:bg-white/10 data-[highlighted]:text-white data-[disabled]:cursor-not-allowed data-[disabled]:opacity-40",
                )}
              >
                <BaseSelect.ItemText>{opt.label}</BaseSelect.ItemText>
                <BaseSelect.ItemIndicator className="text-indigo-300">✓</BaseSelect.ItemIndicator>
              </BaseSelect.Item>
            ))}
          </BaseSelect.Popup>
        </BaseSelect.Positioner>
      </BaseSelect.Portal>
    </BaseSelect.Root>
  );
}

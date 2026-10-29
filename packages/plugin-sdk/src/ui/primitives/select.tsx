import { forwardRef, type ReactNode } from "react";
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
export interface SelectProps {
  value?: string;
  onValueChange?: (value: string) => void;
  options: SelectOption[];
  placeholder?: ReactNode;
  disabled?: boolean;
  className?: string;
  id?: string;
  name?: string;
  // The trigger is a <button>, which <label for> cannot name, so a visible
  // sibling label has to be repeated here.
  "aria-label"?: string;
}

export const Select = forwardRef<HTMLButtonElement, SelectProps>(
  ({ value, onValueChange, options, placeholder, disabled, className, id, name, "aria-label": ariaLabel }, ref) => {
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
          ref={ref}
          aria-label={ariaLabel}
          className={cn(
            "flex w-full cursor-pointer items-center justify-between gap-2 rounded-[var(--radius)] border border-input bg-card px-3.5 text-[13.5px] text-foreground shadow-[inset_0_1px_1px_rgb(0_0_0/0.02)] outline-none transition-all duration-[var(--dur-fast)] ease-[var(--ease-out)]",
            "h-10 py-0",
            "hover:border-border-strong focus-visible:border-accent-border focus-visible:ring-2 focus-visible:ring-accent-soft",
            "data-[popup-open]:border-accent-border disabled:cursor-not-allowed disabled:opacity-50",
            className,
          )}
        >
          <BaseSelect.Value placeholder={placeholder} />
          <BaseSelect.Icon className="text-muted-foreground">▾</BaseSelect.Icon>
        </BaseSelect.Trigger>
        <BaseSelect.Portal>
          <BaseSelect.Positioner sideOffset={6} className="z-50 outline-none" alignItemWithTrigger={false}>
            <BaseSelect.Popup
              className={cn(
                "glass-strong max-h-[min(24rem,var(--available-height))] min-w-[var(--anchor-width)] overflow-y-auto rounded-xl p-1.5 outline-none",
                "origin-[var(--transform-origin)]",
              )}
            >
              {options.map((opt) => (
                <BaseSelect.Item
                  key={opt.value}
                  value={opt.value}
                  disabled={opt.disabled}
                  className={cn(
                    "flex cursor-pointer items-center justify-between gap-3 rounded-lg px-2.5 py-2 text-[13px] text-foreground/80 outline-none transition-colors",
                    "data-[highlighted]:bg-surface-hover data-[highlighted]:text-foreground data-[disabled]:cursor-not-allowed data-[disabled]:opacity-40",
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
  },
);
Select.displayName = "Select";

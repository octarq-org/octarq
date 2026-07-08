import { ReactNode } from "react";
import { Tabs as BaseTabs } from "@base-ui/react/tabs";
import { cn } from "../cn";

export interface TabItem {
  value: string;
  label: ReactNode;
  content: ReactNode;
  disabled?: boolean;
}

// Tabs wraps Base UI's accessible Tabs primitive (roving tabindex, arrow-key
// navigation, aria wiring) with octarq's glass theme and an animated active
// Indicator. Pass `items`, each supplying its own label + panel content.
export function Tabs({
  value,
  defaultValue,
  onValueChange,
  items,
  className,
}: {
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  items: TabItem[];
  className?: string;
}) {
  return (
    <BaseTabs.Root
      value={value}
      defaultValue={defaultValue ?? items[0]?.value}
      onValueChange={(v) => onValueChange?.(String(v))}
      className={className}
    >
      <BaseTabs.List className="relative flex items-center gap-1 rounded-xl bg-white/[0.04] p-1">
        {items.map((it) => (
          <BaseTabs.Tab
            key={it.value}
            value={it.value}
            disabled={it.disabled}
            className={cn(
              "relative z-10 flex-1 rounded-lg px-3 py-1.5 text-sm font-medium outline-none transition-colors",
              "text-white/55 hover:text-white/80 focus-visible:ring-2 focus-visible:ring-indigo-400/60",
              "data-[selected]:text-white disabled:cursor-not-allowed disabled:opacity-40",
            )}
          >
            {it.label}
          </BaseTabs.Tab>
        ))}
        <BaseTabs.Indicator
          className={cn(
            "absolute left-0 top-1 z-0 h-[calc(100%-0.5rem)] rounded-lg bg-white/10 transition-all duration-200",
            "w-[var(--active-tab-width)] translate-x-[var(--active-tab-left)]",
          )}
        />
      </BaseTabs.List>
      {items.map((it) => (
        <BaseTabs.Panel key={it.value} value={it.value} className="mt-4 outline-none">
          {it.content}
        </BaseTabs.Panel>
      ))}
    </BaseTabs.Root>
  );
}

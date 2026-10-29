import { forwardRef, type ReactNode } from "react";
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
export interface TabsProps {
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  items: TabItem[];
  className?: string;
}

export const Tabs = forwardRef<HTMLDivElement, TabsProps>(
  ({ value, defaultValue, onValueChange, items, className }, ref) => {
    return (
      <BaseTabs.Root ref={ref}value={value}
        defaultValue={defaultValue ?? items[0]?.value}
        onValueChange={(v) => onValueChange?.(String(v))}
        className={className}
      >
        <BaseTabs.List className="relative flex items-center gap-1 rounded-xl border border-border bg-well p-1">
          {items.map((it) => (
            <BaseTabs.Tab
              key={it.value}
              value={it.value}
              disabled={it.disabled}
              className={cn(
                "relative z-10 flex-1 cursor-pointer rounded-lg px-3.5 py-2 text-[13px] font-medium outline-none transition-colors duration-[var(--dur-fast)]",
                "text-muted-foreground hover:text-foreground/80 focus-visible:ring-2 focus-visible:ring-ring/60",
                "data-selected:text-foreground disabled:cursor-not-allowed disabled:opacity-40",
              )}
            >
              {it.label}
            </BaseTabs.Tab>
          ))}
          <BaseTabs.Indicator
            className={cn(
              "absolute left-0 top-1 z-0 h-[calc(100%-0.5rem)] rounded-lg border border-border bg-card shadow-[0_1px_2px_rgb(0_0_0/0.05)] transition-all duration-[var(--dur-base)] ease-[var(--ease-out)]",
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
  },
);
Tabs.displayName = "Tabs";

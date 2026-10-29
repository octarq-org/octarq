import { forwardRef, type KeyboardEvent, type ReactNode } from "react";
import { cn } from "../cn";

// TabBar is the presentational tab strip used by pages that own their own
// panels (a route-level `?tab=` param, a list pane that re-renders). SDK Tabs
// is the right choice when the component also owns the panel content; TabBar
// exists for the controlled case, so a page does not hand-roll a fifth
// underline variant. It is the same segmented surface as Tabs' list.
export interface TabBarItem {
  value: string;
  label: ReactNode;
  icon?: ReactNode;
  count?: ReactNode;
  disabled?: boolean;
  testId?: string;
}

export interface TabBarProps {
  items: TabBarItem[];
  value: string;
  onValueChange: (value: string) => void;
  ariaLabel?: string;
  className?: string;
  size?: "sm" | "md";
}

export const TabBar = forwardRef<HTMLDivElement, TabBarProps>(
  ({ items, value, onValueChange, ariaLabel, className, size = "md" }, ref) => {
    // Arrow keys move focus between tabs, Home/End jump to the ends. A
    // controlled strip still needs that keyboard contract; without it the tabs
    // are announceable but not operable from the keyboard.
    function onKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
      const last = items.length - 1;
      let next = -1;
      if (e.key === "ArrowRight") next = index === last ? 0 : index + 1;
      else if (e.key === "ArrowLeft") next = index === 0 ? last : index - 1;
      else if (e.key === "Home") next = 0;
      else if (e.key === "End") next = last;
      if (next < 0) return;
      e.preventDefault();
      const buttons = e.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]');
      buttons?.[next]?.focus();
      if (!items[next]?.disabled) onValueChange(items[next].value);
    }

    return (
      <div
        ref={ref}
        role="tablist"
        aria-label={ariaLabel}
        className={cn(
          "inline-flex max-w-full items-center gap-1 overflow-x-auto rounded-xl border border-border bg-well p-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden",
          className,
        )}
      >
        {items.map((item, index) => {
          const selected = item.value === value;
          return (
            <button
              key={item.value}
              type="button"
              role="tab"
              aria-selected={selected}
              disabled={item.disabled}
              tabIndex={selected ? 0 : -1}
              data-testid={item.testId}
              onClick={() => onValueChange(item.value)}
              onKeyDown={(e) => onKeyDown(e, index)}
              className={cn(
                "flex shrink-0 cursor-pointer items-center gap-2 whitespace-nowrap rounded-lg font-medium outline-none transition-[background-color,color,box-shadow] duration-[var(--dur-fast)] ease-[var(--ease-out)] focus-visible:ring-2 focus-visible:ring-accent-border disabled:cursor-not-allowed disabled:opacity-40",
                size === "sm" ? "px-2.5 py-1 text-[12px]" : "px-3.5 py-2 text-[13px]",
                selected
                  ? "bg-card text-foreground shadow-[0_1px_2px_rgb(0_0_0/0.06)] ring-1 ring-border"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {item.icon && (
                <span className={cn("shrink-0", selected && "text-accent-fg")} aria-hidden="true">
                  {item.icon}
                </span>
              )}
              <span>{item.label}</span>
              {item.count !== undefined && (
                <span
                  className={cn(
                    "rounded-full px-1.5 py-0.5 font-mono text-[10px] leading-none",
                    selected ? "bg-accent-soft text-accent-fg" : "bg-muted text-muted-foreground",
                  )}
                >
                  {item.count}
                </span>
              )}
            </button>
          );
        })}
      </div>
    );
  },
);
TabBar.displayName = "TabBar";

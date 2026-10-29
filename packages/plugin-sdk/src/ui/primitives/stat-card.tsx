import { forwardRef, type ReactNode } from "react";
import { motion } from "framer-motion";
import { cn } from "../cn";

// StatCard is the dashboard metric tile: a value with an optional delta and
// icon, animated in on mount (staggered by `index`) and optionally clickable.
export interface StatCardProps {
  label: string;
  value: string | number;
  delta?: string;
  positive?: boolean;
  icon?: ReactNode;
  index?: number;
  onClick?: () => void;
  className?: string;
}

export const StatCard = forwardRef<HTMLDivElement, StatCardProps>(
  ({ label, value, delta, positive, icon, index = 0, onClick, className }, ref) => (
    <motion.div
      ref={ref}
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.45, delay: index * 0.06, ease: [0.16, 1, 0.3, 1] }}
      onClick={onClick}
      // When clickable, expose button semantics so it's keyboard-operable
      // (Enter/Space) and focus-visible — not just mouse-clickable.
      role={onClick ? "button" : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={
        onClick
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onClick();
              }
            }
          : undefined
      }
      className={cn(
        "group relative overflow-hidden rounded-xl border border-border bg-card p-5 shadow-[var(--shadow-card)] transition-[transform,border-color,box-shadow] duration-[var(--dur-base)] ease-[var(--ease-out)]",
        onClick
          ? "cursor-pointer hover:-translate-y-0.5 hover:border-border-strong hover:shadow-[var(--shadow-float)] focus:outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--ring) active:scale-[0.99]"
          : "",
        className,
      )}
    >
      <div className="mb-4 flex items-start justify-between gap-3">
        <span className="text-[11px] font-medium uppercase tracking-[0.09em] text-muted-foreground">
          {label}
        </span>
        {icon && (
          <span
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-accent-fg transition-transform duration-[var(--dur-base)] ease-[var(--ease-out)] group-hover:scale-105"
            aria-hidden="true"
          >
            {icon}
          </span>
        )}
      </div>
      <div className="flex items-end gap-2.5">
        <span className="tnum font-display text-[1.75rem] font-semibold leading-none tracking-[-0.03em] text-foreground">
          {value}
        </span>
        {delta && (
          <span
            className={cn(
              "mb-0.5 rounded-full px-1.5 py-0.5 text-[11px] font-semibold leading-none",
              positive === true && "bg-success-bg text-success-fg",
              positive === false && "bg-danger-bg text-danger-fg",
              positive === undefined && "bg-muted text-muted-foreground",
            )}
          >
            {delta}
          </span>
        )}
      </div>
    </motion.div>
  ),
);
StatCard.displayName = "StatCard";

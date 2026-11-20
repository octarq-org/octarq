import { forwardRef, type ReactNode } from "react";
import { motion } from "framer-motion";
import { cn } from "../cn";

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
  ({ label, value, delta, positive, icon, onClick, className }, ref) => (
    <motion.div
      ref={ref}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.15, ease: "easeOut" }}
      onClick={onClick}
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
        "group relative overflow-hidden rounded-lg border border-border bg-card p-5 transition-colors duration-100 ease-out",
        onClick
          ? "cursor-pointer hover:border-border-strong hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          : "",
        className,
      )}
    >
      <div className="mb-3 flex items-start justify-between gap-3">
        <span className="text-xs font-medium text-muted-foreground">
          {label}
        </span>
        {icon && (
          <span
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-border bg-well text-muted-foreground"
            aria-hidden="true"
          >
            {icon}
          </span>
        )}
      </div>
      <div className="flex items-end gap-2.5">
        <span className="tnum text-2xl font-semibold leading-none tracking-tight text-foreground">
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

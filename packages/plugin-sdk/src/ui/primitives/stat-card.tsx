import { ReactNode } from "react";
import { motion } from "framer-motion";
import { cn } from "../cn";

// StatCard is the dashboard metric tile: a value with an optional delta and
// icon, animated in on mount (staggered by `index`) and optionally clickable.
export function StatCard({
  label,
  value,
  delta,
  positive = true,
  icon,
  index = 0,
  onClick,
}: {
  label: string;
  value: string | number;
  delta?: string;
  positive?: boolean;
  icon?: ReactNode;
  index?: number;
  onClick?: () => void;
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3, delay: index * 0.05 }}
      onClick={onClick}
      className={cn(
        "glass rounded-2xl p-4 text-left transition-all duration-150",
        onClick ? "cursor-pointer hover:bg-white/[0.06] active:scale-[0.98]" : "",
      )}
    >
      <div className="mb-2 flex items-center justify-between">
        <span className="text-[12px] font-medium text-white/45">{label}</span>
        {icon && <span className="text-white/40">{icon}</span>}
      </div>
      <div className="flex items-end gap-2">
        <span className="font-display text-2xl font-bold tracking-tight text-white">
          {value}
        </span>
        {delta && (
          <span
            className={`mb-1 text-[12px] font-medium ${positive ? "text-emerald-400" : "text-rose-400"}`}
          >
            {delta}
          </span>
        )}
      </div>
    </motion.div>
  );
}

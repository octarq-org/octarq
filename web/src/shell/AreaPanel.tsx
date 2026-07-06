import { NavLink } from "react-router-dom";
import { motion } from "framer-motion";
import { PanelLeft } from "lucide-react";
import { useTranslation } from "../i18n";
import { Area } from "./areas";

export function AreaPanel({ area, currentPath, onCollapse }: { area: Area; currentPath: string; onCollapse: () => void }) {
  const { t } = useTranslation();
  return (
    <motion.div
      initial={{ opacity: 0, x: -10 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: -10 }}
      transition={{ duration: 0.2 }}
      className="relative z-20 flex h-full w-60 flex-col border-r border-white/[0.06] bg-[#0c0c12]/40 backdrop-blur-xl"
    >
      {/* Header */}
      <div className="flex items-start justify-between gap-2 px-4 pb-3 pt-4">
        <div className="min-w-0">
          <h2 className="font-display text-[17px] font-bold tracking-tight text-white truncate">{t(`areas.${area.id}.title`, area.title)}</h2>
          <p className="text-[12px] text-white/45 truncate">{t(`areas.${area.id}.subtitle`, area.subtitle)}</p>
        </div>
        <button
          onClick={onCollapse}
          title={t("app.collapseMenu")}
          className="mt-0.5 shrink-0 rounded-lg p-1.5 text-white/40 transition-colors hover:bg-white/[0.06] hover:text-white"
        >
          <PanelLeft className="h-4 w-4" strokeWidth={1.75} />
        </button>
      </div>

      {/* Grouped nav */}
      <div className="flex-1 overflow-y-auto px-3 pb-3">
        {area.groups.map((group) => (
          <div key={group.label} className="mb-4">
            <p className="px-2 pb-1.5 pt-1 text-[11px] font-semibold uppercase tracking-wider text-white/30">
              {t(`groups.${group.label}`, group.label)}
            </p>
            <div className="space-y-0.5">
              {group.items.map((item) => {
                const active = currentPath.startsWith(item.path);
                return (
                  <NavLink
                    key={item.id}
                    to={item.path}
                    className={`group relative flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-[13px] transition-colors ${
                      active ? "text-white" : "text-white/65 hover:text-white"
                    }`}
                  >
                    {active && (
                      <motion.span
                        layoutId="panel-active"
                        transition={{ type: "spring", stiffness: 500, damping: 40 }}
                        className="absolute inset-0 rounded-xl bg-white/[0.07] ring-1 ring-inset ring-white/10"
                      />
                    )}
                    {item.iconStr ? (
                      <span className={`relative text-sm ${active ? "text-indigo-300" : ""}`}>
                        {item.iconStr}
                      </span>
                    ) : (
                      <item.Icon
                        className={`relative h-[18px] w-[18px] ${active ? "text-indigo-300" : ""}`}
                        strokeWidth={1.75}
                      />
                    )}
                    <span className="relative flex-1">{t(`nav.${item.id}`, item.label)}</span>
                    {item.badge !== undefined && (
                      <span className="relative text-[11px] font-medium text-white/35">
                        {item.badge}
                      </span>
                    )}
                  </NavLink>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </motion.div>
  );
}

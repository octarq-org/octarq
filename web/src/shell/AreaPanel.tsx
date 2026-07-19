import { NavLink } from "react-router-dom";
import { motion } from "framer-motion";
import { useTranslation } from "../i18n";
import { Area } from "./areas";

// AreaPanel renders the second-level navigation for the active area. It is a
// fixed-width (w-60) content block; the collapse animation and the toggle
// affordance live in the shell (App.tsx / TopBar) so this stays presentational
// and the panel width is owned in one place.
export function AreaPanel({
  area,
  currentPath,
  onNavigate,
}: {
  area: Area;
  currentPath: string;
  // Fired when a nav item is chosen — the shell uses it to close the mobile
  // drawer after navigation.
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex h-full w-60 flex-col border-r border-white/[0.06] bg-[#0c0c12]/40 backdrop-blur-xl">
      {/* Header */}
      <div className="px-4 pb-3 pt-4">
        <h2 className="truncate font-display text-[17px] font-bold tracking-tight text-white">
          {t(`areas.${area.id}.title`, area.title)}
        </h2>
        <p className="truncate text-[12px] text-white/50">
          {t(`areas.${area.id}.subtitle`, area.subtitle)}
        </p>
      </div>

      {/* Grouped nav */}
      <div className="flex-1 overflow-y-auto px-3 pb-3 [scrollbar-gutter:stable]">
        {area.groups.map((group) => (
          <div key={group.label} className="mb-4">
            <p className="px-2 pb-1.5 pt-1 text-[11px] font-semibold uppercase tracking-wider text-white/50">
              {t(`groups.${group.label}`, group.label)}
            </p>
            <div className="space-y-0.5">
              {group.items.map((item) => {
                const active = currentPath.startsWith(item.path);
                return (
                  <NavLink
                    key={item.id}
                    to={item.path}
                    onClick={onNavigate}
                    className={`group relative flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-[13px] transition-colors ${
                      active ? "text-white" : "text-white/65 hover:text-white"
                    }`}
                  >
                    {active && (
                      <motion.span
                        layoutId="panel-active"
                        transition={{ type: "spring", stiffness: 500, damping: 40 }}
                        className="absolute inset-0 rounded-xl bg-white/[0.08] ring-1 ring-inset ring-white/10"
                      >
                        {/* Brand-gradient accent bar — the active item carries the
                            same indigo→violet axis as the mark and primary actions. */}
                        <span className="absolute left-0 top-1/2 h-4 w-[3px] -translate-y-1/2 rounded-full bg-gradient-to-b from-indigo-400 to-violet-400" />
                      </motion.span>
                    )}
                    {item.iconStr ? (
                      <span className={`relative text-sm ${active ? "text-indigo-300" : ""}`}>
                        {item.iconStr}
                      </span>
                    ) : (
                      <item.Icon
                        className={`relative h-[18px] w-[18px] ${active ? "text-indigo-300" : "text-white/70 group-hover:text-white"}`}
                        strokeWidth={1.75}
                      />
                    )}
                    <span className="relative flex-1 truncate">{t(`nav.${item.id}`, item.label)}</span>
                    {item.badge !== undefined && (
                      <span className="relative text-[11px] font-medium text-white/50">
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
    </div>
  );
}

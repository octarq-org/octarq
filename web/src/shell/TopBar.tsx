import { useState } from "react";
import { NavLink } from "react-router-dom";
import { AnimatePresence, motion } from "framer-motion";
import { ChevronsUpDown, CheckIcon, Search, Settings, User, CreditCard, LogOut, PanelLeft } from "lucide-react";
import { Org } from "../api";
import { useAppName, brandInitial } from "../brand";
import { useTranslation, LANGS } from "../i18n";
import { Area, AreaId } from "./areas";

export function TopBar({
  areas,
  activeArea,
  settingsActive,
  orgs,
  activeOrgId,
  activeOrgName,
  user,
  showWorkspaceSwitcher,
  panelCollapsed,
  onTogglePanel,
  onSelectArea,
  onSwitchOrg,
  onCreateOrg,
  onOpenSettings,
  onOpenCommand,
  onLogout,
}: {
  areas: Area[];
  activeArea: AreaId;
  settingsActive: boolean;
  orgs: Org[];
  activeOrgId: number;
  activeOrgName: string;
  user: string;
  showWorkspaceSwitcher: boolean;
  panelCollapsed: boolean;
  onTogglePanel: () => void;
  onSelectArea: (id: AreaId) => void;
  onSwitchOrg: (id: number) => void;
  onCreateOrg: () => void;
  onOpenSettings: () => void;
  onOpenCommand: () => void;
  onLogout: () => void;
}) {
  const [wsOpen, setWsOpen] = useState(false);
  const [userOpen, setUserOpen] = useState(false);
  const appName = useAppName();
  const { t, lang, setLang } = useTranslation();

  const initials = activeOrgName
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();
  const userInitials = user.slice(0, 2).toUpperCase();

  return (
    <header className="relative z-30 flex h-14 shrink-0 items-center gap-3 border-b border-white/[0.06] bg-[#07070b]/70 px-3 backdrop-blur-xl">
      {/* Sidebar toggle — always visible so collapse/expand is discoverable
          from one place, independent of which area is open. */}
      <button
        onClick={onTogglePanel}
        aria-label={t("app.collapseMenu")}
        aria-pressed={panelCollapsed}
        title={t("app.collapseMenu")}
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl text-white/55 transition-colors hover:bg-white/5 hover:text-white"
      >
        <PanelLeft
          className={`h-[18px] w-[18px] transition-transform duration-200 ${panelCollapsed ? "rotate-180" : ""}`}
          strokeWidth={1.75}
        />
      </button>

      {/* Brand */}
      <div className="flex items-center gap-2.5 pr-1">
        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow">
          <span className="font-display text-sm font-extrabold text-white">{brandInitial(appName)}</span>
        </div>
        <span className="hidden font-display text-[15px] font-bold tracking-wide text-white sm:block">{appName}</span>
      </div>

      {/* Workspace switcher — Pro only (multi-tenancy) */}
      {showWorkspaceSwitcher && (
      <div className="relative">
        <button
          onClick={() => setWsOpen((v) => !v)}
          aria-label={t("topbar.switchWorkspace")}
          className="flex h-9 items-center gap-2 rounded-xl bg-indigo-500/15 pl-1.5 pr-2 text-xs font-semibold text-indigo-300 ring-1 ring-inset ring-white/10 transition hover:ring-white/25"
        >
          <span className="flex h-6 w-6 items-center justify-center rounded-lg bg-indigo-500/25 text-[10px] font-bold text-indigo-300">
            {initials}
          </span>
          <span className="max-w-[130px] truncate text-sm font-medium text-white/90">{activeOrgName}</span>
          <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 text-white/50" />
        </button>

        <AnimatePresence>
          {wsOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setWsOpen(false)} />
              <motion.div
                initial={{ opacity: 0, scale: 0.95, y: -4 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.95 }}
                transition={{ duration: 0.14 }}
                className="glass-strong absolute left-0 top-11 z-50 w-64 rounded-2xl p-1.5 shadow-2xl"
              >
                <p className="px-2 py-1.5 text-[11px] font-medium uppercase tracking-wide text-white/40">{t("topbar.workspaces")}</p>
                {orgs.map((o) => (
                  <button
                    key={o.id}
                    onClick={() => { onSwitchOrg(o.id); setWsOpen(false); }}
                    className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left transition hover:bg-white/5"
                  >
                    <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/15 text-[11px] font-semibold text-indigo-300 ring-1 ring-inset ring-white/10">
                      {o.name.slice(0, 2).toUpperCase()}
                    </span>
                    <span className="flex-1 truncate text-sm text-white">{o.name}</span>
                    {o.id === activeOrgId && <CheckIcon className="h-4 w-4 text-indigo-400" />}
                  </button>
                ))}
                <div className="my-1 h-px bg-white/[0.06]" />
                <button
                  onClick={() => { onCreateOrg(); setWsOpen(false); }}
                  className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-indigo-300 transition hover:bg-white/5"
                >
                  {t("topbar.newWorkspace")}
                </button>
              </motion.div>
            </>
          )}
        </AnimatePresence>
      </div>
      )}

      {/* Area tabs */}
      <nav className="ml-1 flex items-center gap-1 overflow-x-auto">
        {areas.map((a) => {
          const active = activeArea === a.id && !settingsActive;
          return (
            <button
              key={a.id}
              onClick={() => onSelectArea(a.id)}
              className={`relative flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                active ? "text-white" : "text-white/55 hover:text-white"
              }`}
            >
              {active && (
                <motion.span
                  layoutId="area-tab-active"
                  transition={{ type: "spring", stiffness: 500, damping: 40 }}
                  className="absolute inset-0 rounded-xl bg-white/[0.08] ring-1 ring-inset ring-white/10"
                />
              )}
              <a.Icon className="relative h-4 w-4" strokeWidth={1.75} />
              <span className="relative whitespace-nowrap">{t(`areas.${a.id}.title`, a.title)}</span>
            </button>
          );
        })}
      </nav>

      <div className="flex-1" />

      {/* Command palette trigger */}
      <button
        onClick={onOpenCommand}
        className="flex h-9 items-center gap-2 rounded-xl border border-white/[0.08] bg-white/[0.03] px-2.5 text-white/45 transition-colors hover:bg-white/[0.06] hover:text-white/70"
      >
        <Search className="h-4 w-4" />
        <span className="hidden text-xs md:block">{t("common.search")}</span>
        <kbd className="hidden rounded bg-white/[0.06] px-1.5 py-0.5 text-[10px] font-medium text-white/45 md:block">⌘K</kbd>
      </button>

      {/* Settings */}
      <button
        onClick={onOpenSettings}
        aria-label={t("topbar.settings")}
        title={t("topbar.settings")}
        className={`flex h-9 w-9 items-center justify-center rounded-xl transition-colors ${
          settingsActive ? "bg-white/[0.08] text-white ring-1 ring-inset ring-white/10" : "text-white/55 hover:bg-white/5 hover:text-white"
        }`}
      >
        <Settings className="h-5 w-5" strokeWidth={1.75} />
      </button>

      {/* User menu */}
      <div className="relative">
        <button
          onClick={() => setUserOpen((v) => !v)}
          aria-label={t("topbar.account")}
          className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-indigo-400/30 to-violet-400/30 text-xs font-semibold text-white ring-1 ring-inset ring-white/15 transition hover:ring-white/30"
        >
          {userInitials}
        </button>

        <AnimatePresence>
          {userOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setUserOpen(false)} />
              <motion.div
                initial={{ opacity: 0, scale: 0.95, y: -4 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.95 }}
                transition={{ duration: 0.14 }}
                className="glass-strong absolute right-0 top-11 z-50 w-60 rounded-2xl p-1.5 shadow-2xl"
              >
                <div className="flex items-center gap-2.5 px-2 py-2">
                  <span className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-indigo-400/30 to-violet-400/30 text-xs font-semibold text-white ring-1 ring-inset ring-white/15">
                    {userInitials}
                  </span>
                  <span className="min-w-0">
                    <span className="block truncate text-sm text-white">{user}</span>
                  </span>
                </div>
                <div className="my-1 h-px bg-white/[0.08]" />
                {[
                  { Icon: User, label: t("topbar.personalSettings"), path: "/personal" },
                  { Icon: CreditCard, label: t("topbar.billingPlan"), path: "/settings/billing" },
                ].map((m) => (
                  <NavLink
                    key={m.path}
                    to={m.path}
                    onClick={() => setUserOpen(false)}
                    className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-white/75 transition hover:bg-white/5 hover:text-white"
                  >
                    <m.Icon className="h-4 w-4" />
                    {m.label}
                  </NavLink>
                ))}
                <div className="my-1 h-px bg-white/[0.08]" />
                {/* Language switcher */}
                <div className="flex items-center gap-1 px-2 py-1.5">
                  <span className="mr-auto text-[11px] font-medium uppercase tracking-wide text-white/40">{t("common.language")}</span>
                  {LANGS.map((l) => (
                    <button
                      key={l.code}
                      onClick={() => setLang(l.code)}
                      className={`rounded-lg px-2 py-1 text-xs font-medium transition-colors ${
                        lang === l.code ? "bg-white/[0.1] text-white" : "text-white/50 hover:text-white"
                      }`}
                    >
                      {l.label}
                    </button>
                  ))}
                </div>
                <div className="my-1 h-px bg-white/[0.08]" />
                <button
                  onClick={onLogout}
                  className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-rose-300/90 transition hover:bg-rose-500/10"
                >
                  <LogOut className="h-4 w-4" />
                  {t("common.signOut")}
                </button>
              </motion.div>
            </>
          )}
        </AnimatePresence>
      </div>
    </header>
  );
}

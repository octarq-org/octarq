import { NavLink } from "react-router-dom";
import { motion } from "framer-motion";
import { Menu } from "@base-ui/react/menu";
import { ChevronsUpDown, CheckIcon, Search, Settings, User, CreditCard, LogOut, PanelLeft } from "lucide-react";
import { Org } from "../api";
import { cn } from "../ui";
import { useAppName, brandInitial } from "../brand";
import { useTranslation, LANGS } from "../i18n";
import { Area, AreaId } from "./areas";

// Shared glass styling for the Base UI Menu popups/items used below. Base UI
// gives us Esc-to-close, roving arrow-key focus, focus-return to the trigger,
// outside-click dismissal and portalled positioning — replacing the previous
// hand-rolled `fixed inset-0` overlays.
const MENU_POPUP =
  "glass-strong z-50 origin-[var(--transform-origin)] rounded-2xl p-1.5 shadow-[0_16px_48px_-12px_rgba(0,0,0,0.6)] outline-none " +
  "transition-[transform,opacity] duration-150 data-[starting-style]:scale-95 data-[starting-style]:opacity-0 data-[ending-style]:scale-95 data-[ending-style]:opacity-0";
const MENU_ITEM =
  "flex w-full cursor-pointer items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-white/80 outline-none transition-colors data-[highlighted]:bg-white/10 data-[highlighted]:text-white";

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
        <Menu.Root>
          <Menu.Trigger
            aria-label={t("topbar.switchWorkspace")}
            className="flex h-9 items-center gap-2 rounded-xl bg-indigo-500/15 pl-1.5 pr-2 text-xs font-semibold text-indigo-300 ring-1 ring-inset ring-white/10 transition hover:ring-white/25 data-[popup-open]:ring-white/25"
          >
            <span className="flex h-6 w-6 items-center justify-center rounded-lg bg-indigo-500/25 text-[10px] font-bold text-indigo-300">
              {initials}
            </span>
            <span className="max-w-[130px] truncate text-sm font-medium text-white/90">{activeOrgName}</span>
            <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 text-white/50" />
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Positioner side="bottom" align="start" sideOffset={8} className="z-50 outline-none">
              <Menu.Popup className={cn(MENU_POPUP, "w-64")}>
                <div className="px-2 py-1.5 text-[11px] font-medium uppercase tracking-wide text-white/40">{t("topbar.workspaces")}</div>
                {orgs.map((o) => (
                  <Menu.Item key={o.id} onClick={() => onSwitchOrg(o.id)} className={MENU_ITEM}>
                    <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/15 text-[11px] font-semibold text-indigo-300 ring-1 ring-inset ring-white/10">
                      {o.name.slice(0, 2).toUpperCase()}
                    </span>
                    <span className="flex-1 truncate text-sm text-white">{o.name}</span>
                    {o.id === activeOrgId && <CheckIcon className="h-4 w-4 text-indigo-400" />}
                  </Menu.Item>
                ))}
                <Menu.Separator className="my-1 h-px bg-white/[0.06]" />
                <Menu.Item onClick={onCreateOrg} className={cn(MENU_ITEM, "text-indigo-300")}>
                  {t("topbar.newWorkspace")}
                </Menu.Item>
              </Menu.Popup>
            </Menu.Positioner>
          </Menu.Portal>
        </Menu.Root>
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
      <Menu.Root>
        <Menu.Trigger
          aria-label={t("topbar.account")}
          className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-indigo-400/30 to-violet-400/30 text-xs font-semibold text-white ring-1 ring-inset ring-white/15 transition hover:ring-white/30 data-[popup-open]:ring-white/30"
        >
          {userInitials}
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Positioner side="bottom" align="end" sideOffset={8} className="z-50 outline-none">
            <Menu.Popup className={cn(MENU_POPUP, "w-60")}>
              <div className="flex items-center gap-2.5 px-2 py-2">
                <span className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-indigo-400/30 to-violet-400/30 text-xs font-semibold text-white ring-1 ring-inset ring-white/15">
                  {userInitials}
                </span>
                <span className="block min-w-0 truncate text-sm text-white">{user}</span>
              </div>
              <Menu.Separator className="my-1 h-px bg-white/[0.08]" />
              <Menu.Item render={<NavLink to="/personal" />} className={cn(MENU_ITEM, "text-white/75")}>
                <User className="h-4 w-4" />
                {t("topbar.personalSettings")}
              </Menu.Item>
              <Menu.Item render={<NavLink to="/settings/billing" />} className={cn(MENU_ITEM, "text-white/75")}>
                <CreditCard className="h-4 w-4" />
                {t("topbar.billingPlan")}
              </Menu.Item>
              <Menu.Separator className="my-1 h-px bg-white/[0.08]" />
              {/* Language: a proper radio group so arrow keys move between the
                  segments and the selection is announced; staying open on pick. */}
              <Menu.RadioGroup value={lang} onValueChange={(v) => setLang(v as typeof lang)}>
                <div className="px-2 pb-1 pt-0.5 text-[11px] font-medium uppercase tracking-wide text-white/40">{t("common.language")}</div>
                <div className="flex gap-1 px-1 pb-1">
                  {LANGS.map((l) => (
                    <Menu.RadioItem
                      key={l.code}
                      value={l.code}
                      closeOnClick={false}
                      className="flex-1 cursor-pointer rounded-lg px-2 py-1 text-center text-xs font-medium text-white/50 outline-none transition-colors data-[highlighted]:text-white data-[checked]:bg-white/[0.1] data-[checked]:text-white"
                    >
                      {l.label}
                    </Menu.RadioItem>
                  ))}
                </div>
              </Menu.RadioGroup>
              <Menu.Separator className="my-1 h-px bg-white/[0.08]" />
              <Menu.Item
                onClick={onLogout}
                className="flex w-full cursor-pointer items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-rose-300/90 outline-none transition-colors data-[highlighted]:bg-rose-500/10 data-[highlighted]:text-rose-200"
              >
                <LogOut className="h-4 w-4" />
                {t("common.signOut")}
              </Menu.Item>
            </Menu.Popup>
          </Menu.Positioner>
        </Menu.Portal>
      </Menu.Root>
    </header>
  );
}

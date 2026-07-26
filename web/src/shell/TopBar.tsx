import { NavLink } from "react-router-dom";
import { motion } from "framer-motion";
import { Menu } from "@base-ui/react/menu";
import { Search, Settings, User, LogOut, PanelLeft, Sun, Moon } from "lucide-react";
import { cn } from "../ui";
import { useAppName } from "../brand";
import { BrandMark } from "./BrandMark";
import { useTranslation, LANGS } from "../i18n";
import { useTheme, toggleTheme } from "../theme";
import { Area, AreaId } from "./areas";
import { translateAreaTitle } from "./navI18n";

// Shared styling for the Base UI Menu popups/items used below. Base UI gives us
// Esc-to-close, roving arrow-key focus, focus-return to the trigger,
// outside-click dismissal and portalled positioning. `glass-strong` is the
// mode-aware popover surface (flat white on light, frosted on dark).
const MENU_POPUP =
  "glass-strong z-50 origin-[var(--transform-origin)] rounded-2xl p-1.5 outline-none " +
  "transition-[transform,opacity] duration-150 data-[starting-style]:scale-95 data-[starting-style]:opacity-0 data-[ending-style]:scale-95 data-[ending-style]:opacity-0";
const MENU_ITEM =
  "flex w-full cursor-pointer items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-foreground/80 outline-none transition-colors data-[highlighted]:bg-surface-hover data-[highlighted]:text-foreground";

export function TopBar({
  areas,
  activeArea,
  settingsActive,
  user,
  panelCollapsed,
  onTogglePanel,
  onSelectArea,
  onOpenSettings,
  onOpenCommand,
  onLogout,
}: {
  areas: Area[];
  activeArea: AreaId;
  settingsActive: boolean;
  user: string;
  panelCollapsed: boolean;
  onTogglePanel: () => void;
  onSelectArea: (id: AreaId) => void;
  onOpenSettings: () => void;
  onOpenCommand: () => void;
  onLogout: () => void;
}) {
  const appName = useAppName();
  const { t, lang, setLang } = useTranslation();
  const theme = useTheme();

  const userInitials = user.slice(0, 2).toUpperCase();

  return (
    <header className="relative z-30 flex h-14 shrink-0 items-center gap-3 border-b border-border bg-background/70 px-3 backdrop-blur-xl">
      {/* Menu toggle — mobile only. It opens the overlay drawer on small
          screens; on desktop the collapse control lives in the rail's footer,
          so this is hidden at md+ to avoid two toggles for one state. */}
      <button
        onClick={onTogglePanel}
        aria-label={t("app.collapseMenu")}
        aria-pressed={!panelCollapsed}
        title={t("app.collapseMenu")}
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground md:hidden"
      >
        <PanelLeft className="h-[18px] w-[18px]" strokeWidth={1.75} />
      </button>

      {/* Brand */}
      <div className="flex items-center gap-2.5 pr-1">
        <BrandMark size="sm" />
        <span className="hidden font-display text-[15px] font-bold tracking-wide text-foreground sm:block">{appName}</span>
      </div>

      {/* Area tabs */}
      <nav className="ml-1 flex items-center gap-1 overflow-x-auto">
        {areas.map((a) => {
          const active = activeArea === a.id && !settingsActive;
          return (
            <button
              key={a.id}
              onClick={() => onSelectArea(a.id)}
              className={`relative flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                active ? "text-foreground" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {active && (
                <motion.span
                  layoutId="area-tab-active"
                  transition={{ type: "spring", stiffness: 500, damping: 40 }}
                  className="absolute inset-0 rounded-xl bg-foreground/[0.06] ring-1 ring-inset ring-border"
                />
              )}
              <a.Icon className="relative h-4 w-4" strokeWidth={1.75} />
              <span className="relative whitespace-nowrap">{translateAreaTitle(t, a.id, a.title)}</span>
            </button>
          );
        })}
      </nav>

      <div className="flex-1" />

      {/* Command palette trigger */}
      <button
        onClick={onOpenCommand}
        className="flex h-9 items-center gap-2 rounded-xl border border-border bg-card px-2.5 text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
      >
        <Search className="h-4 w-4" />
        <span className="hidden text-xs md:block">{t("common.search")}</span>
        <kbd className="hidden rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground md:block">⌘K</kbd>
      </button>

      {/* Theme toggle — light is the default (Wise/CF); flips to the frosted dark theme. */}
      <button
        onClick={toggleTheme}
        aria-label={theme === "dark" ? t("topbar.lightMode", "Light mode") : t("topbar.darkMode", "Dark mode")}
        title={theme === "dark" ? t("topbar.lightMode", "Light mode") : t("topbar.darkMode", "Dark mode")}
        className="flex h-9 w-9 items-center justify-center rounded-xl text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
      >
        {theme === "dark" ? <Sun className="h-5 w-5" strokeWidth={1.75} /> : <Moon className="h-5 w-5" strokeWidth={1.75} />}
      </button>

      {/* Settings */}
      <button
        onClick={onOpenSettings}
        aria-label={t("topbar.settings")}
        title={t("topbar.settings")}
        className={`flex h-9 w-9 items-center justify-center rounded-xl transition-colors ${
          settingsActive ? "bg-foreground/[0.06] text-foreground ring-1 ring-inset ring-border" : "text-muted-foreground hover:bg-surface-hover hover:text-foreground"
        }`}
      >
        <Settings className="h-5 w-5" strokeWidth={1.75} />
      </button>

      {/* User menu */}
      <Menu.Root>
        <Menu.Trigger
          aria-label={t("topbar.account")}
          className="flex h-9 w-9 items-center justify-center rounded-full brand-gradient text-xs font-semibold text-white ring-1 ring-inset ring-white/20 transition hover:brightness-110 data-[popup-open]:brightness-110"
        >
          {userInitials}
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Positioner side="bottom" align="end" sideOffset={8} className="z-50 outline-none">
            <Menu.Popup className={cn(MENU_POPUP, "w-60")}>
              <div className="flex items-center gap-2.5 px-2 py-2">
                <span className="flex h-9 w-9 items-center justify-center rounded-full brand-gradient text-xs font-semibold text-white ring-1 ring-inset ring-white/20">
                  {userInitials}
                </span>
                <span className="block min-w-0 truncate text-sm text-foreground">{user}</span>
              </div>
              <Menu.Separator className="my-1 h-px bg-border" />
              <Menu.Item render={<NavLink to="/settings/profile" />} className={cn(MENU_ITEM, "text-foreground/75")}>
                <User className="h-4 w-4" />
                {t("topbar.personalSettings")}
              </Menu.Item>
              <Menu.Separator className="my-1 h-px bg-border" />
              {/* Language: a proper radio group so arrow keys move between the
                  segments and the selection is announced; staying open on pick. */}
              <Menu.RadioGroup value={lang} onValueChange={(v) => setLang(v as typeof lang)}>
                <div className="px-2 pb-1 pt-0.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{t("common.language")}</div>
                <div className="grid grid-cols-2 gap-1 px-1 pb-1">
                  {LANGS.map((l) => (
                    <Menu.RadioItem
                      key={l.code}
                      value={l.code}
                      closeOnClick={false}
                      className="cursor-pointer rounded-lg px-2 py-1 text-center text-xs font-medium text-muted-foreground outline-none transition-colors data-[highlighted]:text-foreground data-[checked]:bg-foreground/[0.08] data-[checked]:text-foreground"
                    >
                      {l.label}
                    </Menu.RadioItem>
                  ))}
                </div>
              </Menu.RadioGroup>
              <Menu.Separator className="my-1 h-px bg-border" />
              <Menu.Item
                onClick={onLogout}
                className="flex w-full cursor-pointer items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-danger-fg outline-none transition-colors data-[highlighted]:bg-danger-fg/10 data-[highlighted]:text-danger-fg"
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

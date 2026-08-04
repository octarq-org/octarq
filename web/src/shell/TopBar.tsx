import { useMemo } from "react";
import { NavLink, useNavigate } from "react-router-dom";
import { motion } from "framer-motion";
import { Menu } from "@base-ui/react/menu";
import { Search, Settings, User, LogOut, PanelLeft, Sun, Moon, Globe, BookOpen, ExternalLink, Info, Plus } from "lucide-react";
import { Action } from "../api";
import { cn } from "../ui";
import { useAppName } from "../brand";
import { BrandMark } from "./BrandMark";
import { useTranslation, LANGS } from "../i18n";
import { useTheme, toggleTheme } from "../theme";
import { Area, AreaId, menuIcon } from "./areas";
import { translateAreaTitle, translateGroupLabel, translateNavItemLabel } from "./navI18n";

const RESOURCES = {
  docs: "https://octarq.org/what-is-octarq/",
  about: "https://octarq.org",
  github: "https://github.com/octarq-org/octarq",
};

function GithubIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
    </svg>
  );
}


// Shared Base UI Menu popover styling.
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
  actions = [],
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
  actions?: Action[];
}) {
  const appName = useAppName();
  const { t, lang, setLang } = useTranslation();
  const theme = useTheme();
  const navigate = useNavigate();

  const userInitials = user.slice(0, 2).toUpperCase();

  const groupedActions = useMemo(() => {
    if (!actions || actions.length === 0) return [];
    const map = new Map<string, Action[]>();
    for (const a of actions) {
      const cat = a.category || "";
      if (!map.has(cat)) map.set(cat, []);
      map.get(cat)!.push(a);
    }
    const res: { category: string; items: Action[] }[] = [];
    for (const [category, items] of map.entries()) {
      items.sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
      res.push({ category, items });
    }
    return res;
  }, [actions]);

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

      {/* Global create menu (+) */}
      {actions.length > 0 && (
        <Menu.Root>
          <Menu.Trigger
            aria-label={t("topbar.create", "Create")}
            title={t("topbar.create", "Create")}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-foreground/10 dark:border-white/10 bg-surface-hover/50 hover:bg-surface-hover hover:border-foreground/20 text-muted-foreground hover:text-foreground transition-all shadow-2xs data-[popup-open]:bg-surface-hover data-[popup-open]:text-foreground"
          >
            <Plus className="h-4 w-4" strokeWidth={1.75} />
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Positioner side="bottom" align="end" sideOffset={8} className="z-50 outline-none">
              <Menu.Popup className={cn(MENU_POPUP, "w-52")}>
                {groupedActions.map((group, gIdx) => (
                  <div key={group.category || gIdx}>
                    {gIdx > 0 && <Menu.Separator className="my-1 h-px bg-border" />}
                    {group.category && (
                      <div className="px-2 pb-1 pt-1.5 text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">
                        {translateGroupLabel(t, group.category)}
                      </div>
                    )}
                    {group.items.map((act) => {
                      const IconComp = menuIcon(act.icon);
                      return (
                        <Menu.Item
                          key={act.id}
                          onClick={() => navigate(act.path)}
                          className={MENU_ITEM}
                        >
                          {IconComp ? (
                            <IconComp className="h-4 w-4 shrink-0 text-muted-foreground" strokeWidth={1.75} />
                          ) : (
                            <span className="w-4 text-center text-sm">{act.icon}</span>
                          )}
                          <span className="flex-1 truncate">{translateNavItemLabel(t, act.id, act.label)}</span>
                        </Menu.Item>
                      );
                    })}
                  </div>
                ))}
              </Menu.Popup>
            </Menu.Positioner>
          </Menu.Portal>
        </Menu.Root>
      )}

      {/* Command palette trigger */}
      <button
        onClick={onOpenCommand}
        className="flex h-9 items-center gap-2 rounded-xl border border-foreground/10 dark:border-white/10 bg-surface-hover/50 hover:bg-surface-hover hover:border-foreground/20 px-3 text-muted-foreground transition-all shadow-2xs group"
      >
        <Search className="h-3.5 w-3.5 text-muted-foreground group-hover:text-primary transition-colors" />
        <span className="hidden text-xs font-medium md:block group-hover:text-foreground">{t("common.search")}</span>
        <kbd className="hidden rounded-md border border-foreground/10 dark:border-white/10 bg-muted/80 px-1.5 py-0.5 text-[10px] font-mono font-medium text-muted-foreground md:block">⌘K</kbd>
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

      {/* Language Switcher Dropdown */}
      <Menu.Root>
        <Menu.Trigger
          aria-label={t("common.language")}
          title={t("common.language")}
          className="flex h-9 items-center gap-1.5 rounded-xl border border-foreground/10 dark:border-white/10 bg-surface-hover/50 hover:bg-surface-hover hover:border-foreground/20 px-2.5 text-xs font-semibold text-muted-foreground transition-all shadow-2xs data-[popup-open]:bg-surface-hover data-[popup-open]:text-foreground"
        >
          <Globe className="h-4 w-4 stroke-[1.75]" />
          <span className="uppercase">{lang}</span>
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Positioner side="bottom" align="end" sideOffset={8} className="z-50 outline-none">
            <Menu.Popup className={cn(MENU_POPUP, "w-40")}>
              <div className="px-2 pb-1.5 pt-1 text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80 flex items-center gap-1.5">
                <Globe className="w-3 h-3 text-primary" />
                {t("common.language")}
              </div>
              <Menu.RadioGroup value={lang} onValueChange={(v) => setLang(v as typeof lang)}>
                <div className="space-y-0.5">
                  {LANGS.map((l) => (
                    <Menu.RadioItem
                      key={l.code}
                      value={l.code}
                      closeOnClick={true}
                      className="flex items-center justify-between cursor-pointer rounded-lg px-2.5 py-1.5 text-xs font-medium text-muted-foreground outline-none transition-colors data-[highlighted]:bg-surface-hover data-[highlighted]:text-foreground data-[checked]:bg-primary/10 data-[checked]:text-primary data-[checked]:font-bold"
                    >
                      <span>{l.label}</span>
                      {lang === l.code && <span className="h-1.5 w-1.5 rounded-full bg-primary" />}
                    </Menu.RadioItem>
                  ))}
                </div>
              </Menu.RadioGroup>
            </Menu.Popup>
          </Menu.Positioner>
        </Menu.Portal>
      </Menu.Root>

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
              <Menu.Item render={<a href={RESOURCES.github} target="_blank" rel="noreferrer" />} className={cn(MENU_ITEM, "text-foreground/75")}>
                <GithubIcon className="h-4 w-4" />
                <span className="flex-1">{t("footer.github", "GitHub")}</span>
                <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              </Menu.Item>
              <Menu.Item render={<a href={RESOURCES.about} target="_blank" rel="noreferrer" />} className={cn(MENU_ITEM, "text-foreground/75")}>
                <Info className="h-4 w-4" />
                <span className="flex-1">{t("footer.about", "关于 Octarq")}</span>
                <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              </Menu.Item>
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

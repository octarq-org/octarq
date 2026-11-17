import { cn } from "@octarq/plugin-sdk";
import { useState } from "react";
import { NavLink } from "react-router-dom";
import { Menu } from "@base-ui/react/menu";
import {
  ArrowLeft,
  BookOpen,
  CheckIcon,
  ChevronDown,
  ChevronsUpDown,
  ExternalLink,
  Globe,
  HelpCircle,
  Info,
  LogOut,
  MessageCircle,
  Moon,
  PanelLeft,
  PanelLeftClose,
  Plus,
  Settings,
  Sun,
  User,
} from "lucide-react";
import { Org } from "../api";
import { useAppName } from "../brand";
import { LANGS, useTranslation } from "../i18n";
import { toggleTheme, useTheme } from "../theme";
import { Area, NavGroup, NavItem } from "./areas";
import { BrandMark } from "./BrandMark";
import { MENU_ITEM, MENU_POPUP } from "./menuStyles";
import { isNavItemActive } from "./navActive";
import {
  translateAreaTitle,
  translateGroupLabel,
  translateNavItemLabel,
} from "./navI18n";
import { RESOURCES } from "./resources";

function GithubIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
    </svg>
  );
}

function SidebarNavRow({
  item,
  currentPath,
  collapsed,
  onNavigate,
}: {
  item: NavItem;
  currentPath: string;
  collapsed: boolean;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  const active = isNavItemActive(item.path, currentPath);
  const label = translateNavItemLabel(t, item.id, item.label);

  if (collapsed) {
    if (item.external) {
      return (
        <a
          href={item.path}
          onClick={onNavigate}
          title={label}
          aria-label={label}
          className="relative mx-auto flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors duration-100 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground pointer-coarse:h-11 pointer-coarse:w-11"
        >
          <ExternalLink className="h-4 w-4" strokeWidth={1.75} />
        </a>
      );
    }
    return (
      <NavLink
        to={item.path}
        onClick={onNavigate}
        title={label}
        aria-label={label}
        className={cn(
          "relative mx-auto flex h-9 w-9 items-center justify-center rounded-md transition-colors duration-100 pointer-coarse:h-11 pointer-coarse:w-11",
          active
            ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
            : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
        )}
      >
        {item.iconStr ? (
          <span className="text-sm">{item.iconStr}</span>
        ) : (
          <item.Icon className="h-4 w-4" strokeWidth={1.75} />
        )}
        {item.badge !== undefined && (
          <span className="absolute right-1 top-1 flex h-1.5 w-1.5 rounded-full bg-primary" />
        )}
      </NavLink>
    );
  }

  if (item.external) {
    return (
      <a
        href={item.path}
        onClick={onNavigate}
        className="group flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-[13px] text-muted-foreground transition-colors duration-100 hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground pointer-coarse:min-h-11"
      >
        <ExternalLink className="h-4 w-4 shrink-0 text-muted-foreground group-hover:text-sidebar-accent-foreground" strokeWidth={1.75} />
        <span className="flex-1 truncate">{label}</span>
      </a>
    );
  }

  return (
    <NavLink
      to={item.path}
      onClick={onNavigate}
      className={cn(
        "group flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-[13px] transition-colors duration-100 pointer-coarse:min-h-11",
        active
          ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
          : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
      )}
    >
      {item.iconStr ? (
        <span className={cn("w-4 shrink-0 text-center text-sm", active && "text-accent-fg")}>
          {item.iconStr}
        </span>
      ) : (
        <item.Icon
          className={cn(
            "h-4 w-4 shrink-0",
            active ? "text-accent-fg" : "text-muted-foreground group-hover:text-sidebar-accent-foreground",
          )}
          strokeWidth={1.75}
        />
      )}
      <span className="flex-1 truncate">{label}</span>
      {item.badge !== undefined && (
        <span className="text-[11px] font-medium text-muted-foreground">
          {item.badge}
        </span>
      )}
    </NavLink>
  );
}

function SidebarGroupList({
  groups,
  currentPath,
  collapsed,
  onNavigate,
}: {
  groups: NavGroup[];
  currentPath: string;
  collapsed: boolean;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {groups.map((group, gi) => (
        <div key={group.label} className={collapsed ? "mb-1" : "mb-3"}>
          {collapsed ? (
            gi > 0 && <div className="mx-auto my-1.5 h-px w-6 bg-sidebar-border" />
          ) : (
            <p className="px-2.5 pb-1 pt-2.5 text-[11px] font-medium text-muted-foreground/80">
              {translateGroupLabel(t, group.label)}
            </p>
          )}
          <div className="space-y-0.5">
            {group.items.map((item) => (
              <SidebarNavRow
                key={item.id}
                item={item}
                currentPath={currentPath}
                collapsed={collapsed}
                onNavigate={onNavigate}
              />
            ))}
          </div>
        </div>
      ))}
    </>
  );
}

// AreaPanel is the unified single-depth left sidebar sitting directly on the
// Frame surface (`bg-sidebar`). On normal routes it shows every business area
// and its groups in one scrollable hierarchy (eliminating the old 2D Area Tabs).
// On `/settings/*` or `/help/*` it renders the contextual settings/help tree
// with a back link to `/overview`.
export function AreaPanel({
  areas,
  settingsArea,
  helpArea,
  settingsActive,
  helpActive,
  user,
  currentPath,
  collapsed,
  onToggle,
  onNavigate,
  onOpenSettings,
  onLogout,
  footerItems,
  showWorkspaceSwitcher,
  orgs,
  activeOrgId,
  activeOrgName,
  onSwitchOrg,
  onCreateOrg,
}: {
  areas: Area[];
  settingsArea: Area;
  helpArea: Area;
  settingsActive: boolean;
  helpActive: boolean;
  user: string;
  currentPath: string;
  collapsed: boolean;
  onToggle: () => void;
  onNavigate?: () => void;
  onOpenSettings: () => void;
  onLogout: () => void;
  footerItems: NavItem[];
  showWorkspaceSwitcher: boolean;
  orgs: Org[];
  activeOrgId: number;
  activeOrgName: string;
  onSwitchOrg: (id: number) => void;
  onCreateOrg: () => void;
}) {
  const appName = useAppName();
  const { t, lang, setLang } = useTranslation();
  const theme = useTheme();
  const [closedAreas, setClosedAreas] = useState<Record<string, boolean>>({});

  const orgInitials = activeOrgName
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();

  const userInitials = (user || "U").slice(0, 2).toUpperCase();

  const toggleAreaSection = (id: string) => {
    setClosedAreas((prev) => ({ ...prev, [id]: !prev[id] }));
  };

  return (
    <div className="flex h-full w-full flex-col bg-sidebar text-sidebar-foreground">
      {/* ── Header: workspace switcher (Pro) or brand identity ── */}
      <div className={cn("flex h-14 shrink-0 items-center", collapsed ? "justify-center px-2" : "px-3")}>
        {showWorkspaceSwitcher ? (
          <Menu.Root>
            <Menu.Trigger
              aria-label={t("topbar.switchWorkspace")}
              title={collapsed ? activeOrgName : undefined}
              className={cn(
                "tap-expand flex cursor-pointer items-center rounded-md border border-sidebar-border bg-card transition-colors duration-100 hover:border-border-strong data-[popup-open]:border-border-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring",
                collapsed ? "h-8 w-8 justify-center" : "h-8 w-full gap-2 px-2",
              )}
            >
              <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-accent-soft text-[10px] font-semibold text-accent-fg">
                {orgInitials}
              </span>
              {!collapsed && (
                <>
                  <span className="min-w-0 flex-1 truncate text-left text-[13px] font-medium text-foreground">{activeOrgName}</span>
                  <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </>
              )}
            </Menu.Trigger>
            <Menu.Portal>
              <Menu.Positioner side={collapsed ? "right" : "bottom"} align="start" sideOffset={8} className="z-50 outline-none">
                <Menu.Popup className={cn(MENU_POPUP, "w-64")}>
                  <div className="px-2 py-1.5 text-[11px] font-medium text-muted-foreground">{t("topbar.workspaces")}</div>
                  {orgs.map((o) => (
                    <Menu.Item key={o.id} onClick={() => onSwitchOrg(o.id)} className={MENU_ITEM}>
                      <span className="flex h-7 w-7 items-center justify-center rounded-md bg-accent-soft text-[11px] font-semibold text-accent-fg ring-1 ring-inset ring-border">
                        {o.name.slice(0, 2).toUpperCase()}
                      </span>
                      <span className="flex-1 truncate text-sm text-foreground">{o.name}</span>
                      {o.id === activeOrgId && <CheckIcon className="h-4 w-4 text-accent-fg" />}
                    </Menu.Item>
                  ))}
                  <Menu.Separator className="my-1 h-px bg-border" />
                  <Menu.Item onClick={onCreateOrg} className={cn(MENU_ITEM, "text-accent-fg")}>
                    <Plus className="h-4 w-4" />
                    {t("topbar.newWorkspace")}
                  </Menu.Item>
                </Menu.Popup>
              </Menu.Positioner>
            </Menu.Portal>
          </Menu.Root>
        ) : (
          <NavLink
            to="/overview"
            onClick={onNavigate}
            title={collapsed ? appName : undefined}
            className={cn(
              "flex items-center gap-2 rounded-md transition-colors hover:opacity-90",
              collapsed ? "justify-center p-1" : "w-full px-1 py-1",
            )}
          >
            <BrandMark size="sm" className="!h-7 !w-7 !rounded-md" />
            {!collapsed && (
              <span className="truncate text-sm font-semibold tracking-tight text-foreground">{appName}</span>
            )}
          </NavLink>
        )}
      </div>

      {/* ── Unified Navigation Tree ── */}
      <div className={cn("flex-1 overflow-y-auto py-2 [scrollbar-gutter:stable]", collapsed ? "px-2" : "px-2.5")}>
        {settingsActive || helpActive ? (
          <>
            <NavLink
              to="/overview"
              onClick={onNavigate}
              title={collapsed ? t("topbar.backToWorkspace", "Back to workspace") : undefined}
              className={cn(
                "mb-2 flex items-center rounded-md text-muted-foreground transition-colors duration-100 hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
                collapsed ? "mx-auto h-9 w-9 justify-center" : "gap-2 px-2.5 py-1.5 text-[13px] font-medium",
              )}
            >
              <ArrowLeft className="h-4 w-4 shrink-0" strokeWidth={1.75} />
              {!collapsed && <span className="truncate">{t("topbar.backToWorkspace", "Back to workspace")}</span>}
            </NavLink>
            <SidebarGroupList
              groups={(settingsActive ? settingsArea : helpArea).groups}
              currentPath={currentPath}
              collapsed={collapsed}
              onNavigate={onNavigate}
            />
          </>
        ) : areas.length <= 1 ? (
          <SidebarGroupList
            groups={areas[0]?.groups ?? []}
            currentPath={currentPath}
            collapsed={collapsed}
            onNavigate={onNavigate}
          />
        ) : (
          areas.map((area, ai) => {
            const isOpen = !closedAreas[area.id];
            return (
              <div key={area.id} className={cn(ai > 0 && !collapsed && "mt-3")}>
                {!collapsed ? (
                  <button
                    type="button"
                    onClick={() => toggleAreaSection(area.id)}
                    aria-expanded={isOpen}
                    className="flex w-full items-center justify-between rounded-md px-2.5 py-1 text-[11px] font-semibold text-sidebar-foreground/70 transition-colors hover:text-sidebar-foreground"
                  >
                    <span className="truncate">{translateAreaTitle(t, area.id, area.title)}</span>
                    <ChevronDown
                      className={cn(
                        "h-3.5 w-3.5 shrink-0 transition-transform duration-150",
                        !isOpen && "-rotate-90",
                      )}
                    />
                  </button>
                ) : (
                  ai > 0 && <div className="mx-auto my-2 h-px w-6 bg-sidebar-border" />
                )}
                {(collapsed || isOpen) && (
                  <SidebarGroupList
                    groups={area.groups}
                    currentPath={currentPath}
                    collapsed={collapsed}
                    onNavigate={onNavigate}
                  />
                )}
              </div>
            );
          })
        )}
      </div>

      {/* ── Footer: plugin footer items + Settings + Account + Collapse toggle ── */}
      <div className={cn("border-t border-sidebar-border", collapsed ? "space-y-1 p-2" : "space-y-0.5 p-2.5")}>
        {footerItems.map((it) => (
          <SidebarNavRow
            key={it.id}
            item={it}
            currentPath={currentPath}
            collapsed={collapsed}
            onNavigate={onNavigate}
          />
        ))}

        {/* Settings docked in sidebar footer */}
        <button
          type="button"
          onClick={() => {
            onOpenSettings();
            onNavigate?.();
          }}
          aria-label={t("topbar.settings")}
          title={collapsed ? t("topbar.settings") : undefined}
          className={cn(
            "flex cursor-pointer items-center rounded-md transition-colors duration-100",
            settingsActive
              ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
              : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
            collapsed
              ? "mx-auto h-9 w-9 justify-center pointer-coarse:h-11 pointer-coarse:w-11"
              : "h-8 w-full gap-2.5 px-2.5 text-[13px] pointer-coarse:min-h-11",
          )}
        >
          <Settings className={cn("h-4 w-4 shrink-0", settingsActive && "text-accent-fg")} strokeWidth={1.75} />
          {!collapsed && <span className="truncate">{t("topbar.settings")}</span>}
        </button>

        {/* Account & preferences menu */}
        <Menu.Root>
          <Menu.Trigger
            aria-label={t("topbar.account")}
            title={collapsed ? user : undefined}
            className={cn(
              "flex cursor-pointer items-center rounded-md text-muted-foreground transition-colors duration-100 hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground data-[popup-open]:bg-sidebar-accent data-[popup-open]:text-sidebar-accent-foreground",
              collapsed
                ? "mx-auto h-9 w-9 justify-center pointer-coarse:h-11 pointer-coarse:w-11"
                : "h-8 w-full gap-2.5 px-2.5 text-[13px] pointer-coarse:min-h-11",
            )}
          >
            <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full brand-gradient text-[9px] font-semibold text-white">
              {userInitials}
            </span>
            {!collapsed && (
              <>
                <span className="min-w-0 flex-1 truncate text-left text-foreground">{user}</span>
                <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              </>
            )}
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Positioner side={collapsed ? "right" : "top"} align="start" sideOffset={8} className="z-50 outline-none">
              <Menu.Popup className={cn(MENU_POPUP, "w-60")}>
                <div className="flex items-center gap-2.5 px-2.5 py-2">
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full brand-gradient text-[11px] font-semibold text-white">
                    {userInitials}
                  </span>
                  <span className="block min-w-0 truncate text-[13px] font-medium text-foreground">{user}</span>
                </div>
                <Menu.Separator className="my-1 h-px bg-border" />
                <Menu.Item render={<NavLink to="/settings/profile" />} onClick={onNavigate} className={cn(MENU_ITEM, "text-foreground")}>
                  <User className="h-4 w-4" />
                  {t("topbar.personalSettings")}
                </Menu.Item>
                <Menu.Item onClick={toggleTheme} className={cn(MENU_ITEM, "text-foreground")}>
                  {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
                  {theme === "dark" ? t("topbar.lightMode", "Light mode") : t("topbar.darkMode", "Dark mode")}
                </Menu.Item>
                <Menu.SubmenuRoot>
                  <Menu.SubmenuTrigger className={cn(MENU_ITEM, "text-foreground")}>
                    <Globe className="h-4 w-4" />
                    <span className="flex-1">{t("common.language")}</span>
                    <span className="text-[11px] uppercase text-muted-foreground">{lang}</span>
                  </Menu.SubmenuTrigger>
                  <Menu.Portal>
                    <Menu.Positioner side="right" align="start" sideOffset={6} className="z-50 outline-none">
                      <Menu.Popup className={cn(MENU_POPUP, "w-40")}>
                        <Menu.RadioGroup value={lang} onValueChange={(v) => setLang(v as typeof lang)}>
                          {LANGS.map((l) => (
                            <Menu.RadioItem
                              key={l.code}
                              value={l.code}
                              closeOnClick={true}
                              className="flex cursor-pointer items-center justify-between rounded-md px-2.5 py-1.5 text-xs font-medium text-muted-foreground outline-none transition-colors data-[highlighted]:bg-surface-hover data-[highlighted]:text-foreground data-[checked]:bg-accent-soft data-[checked]:text-accent-fg"
                            >
                              <span>{l.label}</span>
                              {lang === l.code && <CheckIcon className="h-3.5 w-3.5" />}
                            </Menu.RadioItem>
                          ))}
                        </Menu.RadioGroup>
                      </Menu.Popup>
                    </Menu.Positioner>
                  </Menu.Portal>
                </Menu.SubmenuRoot>
                <Menu.Separator className="my-1 h-px bg-border" />
                <Menu.Item render={<a href={RESOURCES.docs} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
                  <BookOpen className="h-4 w-4" strokeWidth={1.75} />
                  <span className="flex-1">{t("footer.docs", "Developer Docs")}</span>
                  <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </Menu.Item>
                <Menu.Item render={<a href={RESOURCES.github} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
                  <GithubIcon className="h-4 w-4" />
                  <span className="flex-1">{t("footer.github", "GitHub")}</span>
                  <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </Menu.Item>
                <Menu.Item render={<a href={RESOURCES.about} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
                  <Info className="h-4 w-4" strokeWidth={1.75} />
                  <span className="flex-1">{t("footer.about", "Website")}</span>
                  <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </Menu.Item>
                <Menu.Item render={<a href={RESOURCES.contact} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
                  <MessageCircle className="h-4 w-4" strokeWidth={1.75} />
                  <span className="flex-1">{t("footer.contact", "Contact")}</span>
                  <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </Menu.Item>
                <Menu.Separator className="my-1 h-px bg-border" />
                <Menu.Item
                  onClick={onLogout}
                  className="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-[13px] text-danger-fg outline-none transition-colors data-[highlighted]:bg-danger-bg data-[highlighted]:text-danger-fg"
                >
                  <LogOut className="h-4 w-4" />
                  {t("common.signOut")}
                </Menu.Item>
              </Menu.Popup>
            </Menu.Positioner>
          </Menu.Portal>
        </Menu.Root>

        {/* Collapse rail toggle */}
        <button
          type="button"
          onClick={onToggle}
          aria-label={t("app.collapseMenu")}
          aria-pressed={collapsed}
          title={collapsed ? t("app.collapseMenu") : undefined}
          className={cn(
            "flex cursor-pointer items-center rounded-md text-muted-foreground transition-colors duration-100 hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
            collapsed ? "mx-auto h-9 w-9 justify-center" : "h-8 w-full gap-2.5 px-2.5 text-[13px]",
          )}
        >
          {collapsed ? (
            <PanelLeft className="h-4 w-4" strokeWidth={1.75} />
          ) : (
            <>
              <PanelLeftClose className="h-4 w-4" strokeWidth={1.75} />
              <span>{t("app.collapseMenu")}</span>
            </>
          )}
        </button>
      </div>
    </div>
  );
}

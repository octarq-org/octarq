import { ExtensionSlot, cn } from "@octarq/plugin-sdk";
import { Fragment, useMemo } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { Menu } from "@base-ui/react/menu";
import { Search, PanelLeft, Sun, Moon, Plus, ChevronRight } from "lucide-react";
import { Action } from "../api";
import { useTranslation } from "../i18n";
import { useTheme, toggleTheme } from "../theme";
import { Area, AreaId, menuIcon } from "./areas";
import { resolveBreadcrumbs } from "./breadcrumbs";
import { translateGroupLabel, translateNavItemLabel } from "./navI18n";
import { MENU_ITEM, MENU_POPUP } from "./menuStyles";
import { NotificationBell, InboxDrawer } from "../pages/notifications";
import { AICopilotButton, AICopilotDrawer } from "./AICopilotDrawer";

const ACTION_BUTTON =
  "rounded-md border border-border bg-card text-muted-foreground " +
  "transition-colors duration-100 ease-out " +
  "hover:border-border-strong hover:bg-surface-hover hover:text-foreground " +
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring " +
  "data-[popup-open]:border-border-strong data-[popup-open]:bg-surface-hover data-[popup-open]:text-foreground";

export function TopBar({
  areas,
  settingsArea,
  helpArea,
  panelCollapsed,
  onTogglePanel,
  onOpenCommand,
  actions = [],
}: {
  areas: Area[];
  activeArea?: AreaId;
  settingsActive?: boolean;
  settingsArea?: Area;
  helpArea?: Area;
  user?: string;
  panelCollapsed: boolean;
  onTogglePanel: () => void;
  onSelectArea?: (id: AreaId) => void;
  onOpenSettings?: () => void;
  onOpenCommand: () => void;
  onLogout?: () => void;
  actions?: Action[];
}) {
  const { t } = useTranslation();
  const theme = useTheme();
  const navigate = useNavigate();
  const location = useLocation();

  const crumbs = useMemo(
    () =>
      resolveBreadcrumbs({
        pathname: location.pathname,
        areas,
        settingsArea,
        helpArea,
        t,
      }),
    [location.pathname, areas, settingsArea, helpArea, t],
  );

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
    <header className="relative z-20 flex h-14 shrink-0 items-center gap-2 border-b border-border bg-background px-3 sm:px-4 md:gap-3">
      {/* Sidebar drawer toggle — mobile only (desktop collapse lives on AreaRail) */}
      <button
        type="button"
        onClick={onTogglePanel}
        aria-label={t("app.collapseMenu")}
        aria-pressed={!panelCollapsed}
        title={t("app.collapseMenu")}
        className="tap-expand flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors duration-100 hover:bg-surface-hover hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:hidden"
      >
        <PanelLeft className="h-4 w-4" strokeWidth={1.75} />
      </button>

      {/* Breadcrumb trail */}
      <nav aria-label={t("topbar.breadcrumb", "Breadcrumb")} className="flex min-w-0 items-center gap-1 text-[13px]">
        {crumbs.map((crumb, idx) => {
          const isLast = idx === crumbs.length - 1;
          return (
            <Fragment key={`${crumb.label}-${idx}`}>
              {idx > 0 && <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" aria-hidden="true" />}
              {isLast || !crumb.path ? (
                <span
                  aria-current={isLast ? "page" : undefined}
                  className={cn(
                    "truncate",
                    isLast ? "font-medium text-foreground" : "text-muted-foreground",
                  )}
                >
                  {crumb.label}
                </span>
              ) : (
                <NavLink
                  to={crumb.path}
                  className="truncate text-muted-foreground transition-colors duration-100 hover:text-foreground"
                >
                  {crumb.label}
                </NavLink>
              )}
            </Fragment>
          );
        })}
      </nav>

      {/* Centered Command Palette search bar (also serves as header > .flex-1 spacer) */}
      <div className="flex flex-1 items-center justify-end sm:justify-center">
        <button
          type="button"
          onClick={onOpenCommand}
          aria-label={t("common.search")}
          className={cn(
            ACTION_BUTTON,
            "tap-expand group flex h-8 items-center justify-between gap-2 px-2.5 sm:w-56 md:w-64",
          )}
        >
          <span className="flex items-center gap-2 truncate">
            <Search className="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground" aria-hidden="true" />
            <span className="hidden truncate text-xs sm:inline">{t("common.search")}</span>
          </span>
          <kbd className="hidden rounded border border-border bg-well px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground sm:inline">⌘K</kbd>
        </button>
      </div>

      {/* Plugin extension point — leftmost of the right-hand cluster */}
      <ExtensionSlot name="topbar-right" />

      {/* Global create menu (+) */}
      {actions.length > 0 && (
        <Menu.Root>
          <Menu.Trigger
            aria-label={t("topbar.create", "Create")}
            title={t("topbar.create", "Create")}
            className={cn(ACTION_BUTTON, "tap-expand flex h-8 w-8 items-center justify-center")}
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
                      <div className="px-2 pb-1 pt-1.5 text-[11px] font-medium text-muted-foreground">
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

      {/* Notifications bell icon */}
      <NotificationBell />

      {/* AI Copilot Sparkles trigger (⌘+J) */}
      <AICopilotButton />

      {/* Quick theme toggle */}
      <button
        type="button"
        onClick={toggleTheme}
        aria-label={theme === "dark" ? t("topbar.lightMode", "Light mode") : t("topbar.darkMode", "Dark mode")}
        title={theme === "dark" ? t("topbar.lightMode", "Light mode") : t("topbar.darkMode", "Dark mode")}
        className="tap-expand hidden h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors duration-100 hover:bg-surface-hover hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:flex"
      >
        {theme === "dark" ? <Sun className="h-4 w-4" strokeWidth={1.75} /> : <Moon className="h-4 w-4" strokeWidth={1.75} />}
      </button>

      {/* In-app Notification Inbox Drawer */}
      <InboxDrawer />

      {/* Inline AI Copilot Drawer */}
      <AICopilotDrawer />
    </header>
  );
}

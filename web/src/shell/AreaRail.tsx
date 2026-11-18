import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { NavLink } from "react-router-dom";
import { Menu } from "@base-ui/react/menu";
import { CheckIcon, PanelLeft, PanelLeftClose, Plus, Settings } from "lucide-react";
import { cn } from "@octarq/plugin-sdk";
import { Org } from "../api";
import { useAppName } from "../brand";
import { useTranslation } from "../i18n";
import { Area, AreaId, NavItem } from "./areas";
import { AccountMenu } from "./AccountMenu";
import { BrandMark } from "./BrandMark";
import { MENU_ITEM, MENU_POPUP } from "./menuStyles";
import { isNavItemActive } from "./navActive";
import { translateAreaTitle, translateNavItemLabel } from "./navI18n";
import { SidebarGroupList } from "./SidebarNav";

interface FlyoutState {
  area: Area;
  top?: number;
  bottom?: number;
}

const FLYOUT_OPEN_DELAY_MS = 60;
const FLYOUT_CLOSE_DELAY_MS = 140;

function AreaRailButton({
  active,
  label,
  onClick,
  onMouseEnter,
  onMouseLeave,
  children,
}: {
  active: boolean;
  label: string;
  onClick: () => void;
  onMouseEnter?: (e: React.MouseEvent<HTMLButtonElement>) => void;
  onMouseLeave?: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
      aria-label={label}
      title={label}
      aria-current={active ? "page" : undefined}
      className={cn(
        "relative flex h-9 w-9 cursor-pointer items-center justify-center rounded-md transition-colors duration-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring pointer-coarse:h-11 pointer-coarse:w-11",
        active
          ? "bg-sidebar-accent text-accent-fg"
          : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
      )}
    >
      {active && (
        <span
          aria-hidden="true"
          className="absolute -left-[7px] top-1/2 h-4 w-[3px] -translate-y-1/2 rounded-r-full bg-primary"
        />
      )}
      {children}
    </button>
  );
}

export function AreaRail({
  areas,
  activeArea,
  settingsArea,
  helpArea,
  settingsActive,
  helpActive,
  user,
  currentPath,
  collapsed,
  onToggle,
  onSelectArea,
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
  activeArea: AreaId;
  settingsArea: Area;
  helpArea: Area;
  settingsActive: boolean;
  helpActive: boolean;
  user: string;
  currentPath: string;
  collapsed: boolean;
  onToggle: () => void;
  onSelectArea: (id: AreaId) => void;
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
  const { t } = useTranslation();
  const [flyout, setFlyout] = useState<FlyoutState | null>(null);
  const openTimerRef = useRef<number | null>(null);
  const closeTimerRef = useRef<number | null>(null);

  const orgInitials = activeOrgName
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();

  const clearTimers = () => {
    if (openTimerRef.current !== null) {
      window.clearTimeout(openTimerRef.current);
      openTimerRef.current = null;
    }
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
  };

  useEffect(() => clearTimers, []);

  useEffect(() => {
    if (!collapsed) {
      clearTimers();
      setFlyout(null);
    }
  }, [collapsed]);

  useEffect(() => {
    if (!flyout) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        clearTimers();
        setFlyout(null);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [flyout]);

  const scheduleFlyout = (area: Area, target: HTMLElement) => {
    if (!collapsed) return;
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
    if (openTimerRef.current !== null) {
      window.clearTimeout(openTimerRef.current);
    }
    const rect = target.getBoundingClientRect();
    openTimerRef.current = window.setTimeout(() => {
      openTimerRef.current = null;
      const vh = typeof window !== "undefined" ? window.innerHeight : 900;
      if (rect.top > vh / 2) {
        setFlyout({
          area,
          bottom: Math.max(12, vh - rect.bottom),
        });
      } else {
        setFlyout({
          area,
          top: Math.max(12, rect.top),
        });
      }
    }, FLYOUT_OPEN_DELAY_MS);
  };

  const scheduleClose = () => {
    if (openTimerRef.current !== null) {
      window.clearTimeout(openTimerRef.current);
      openTimerRef.current = null;
    }
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current);
    }
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null;
      setFlyout(null);
    }, FLYOUT_CLOSE_DELAY_MS);
  };

  const cancelClose = () => {
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
  };

  return (
    <div
      data-slot="area-rail"
      className="flex h-full w-[52px] shrink-0 flex-col items-center border-r border-sidebar-border/80 bg-sidebar py-2"
    >
      {/* Top anchor: workspace switcher (Pro) or brand mark (OSS) */}
      <div className="flex h-10 w-full shrink-0 items-center justify-center px-1.5">
        {showWorkspaceSwitcher ? (
          <Menu.Root>
            <Menu.Trigger
              aria-label={t("topbar.switchWorkspace")}
              title={activeOrgName}
              className="tap-expand flex h-8 w-8 cursor-pointer items-center justify-center rounded-md border border-sidebar-border bg-card text-[11px] font-semibold text-accent-fg transition-colors duration-100 hover:border-border-strong data-[popup-open]:border-border-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring"
            >
              {orgInitials}
            </Menu.Trigger>
            <Menu.Portal>
              <Menu.Positioner side="right" align="start" sideOffset={8} className="z-50 outline-none">
                <Menu.Popup className={cn(MENU_POPUP, "w-64")}>
                  <div className="px-2 py-1.5 text-[11px] font-medium text-muted-foreground">
                    {t("topbar.workspaces")}
                  </div>
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
            title={appName}
            aria-label={appName}
            className="flex h-8 w-8 items-center justify-center rounded-md transition-opacity hover:opacity-90"
          >
            <BrandMark size="sm" className="!h-7 !w-7 !rounded-md" />
          </NavLink>
        )}
      </div>

      {/* Primary area icons */}
      <nav
        aria-label={t("topbar.switchArea", "Switch area")}
        className="flex w-full flex-1 flex-col items-center gap-1.5 overflow-y-auto px-1.5 py-3"
      >
        {areas.map((area) => {
          const isCurrent = activeArea === area.id && !settingsActive && !helpActive;
          const title = translateAreaTitle(t, area.id, area.title);
          return (
            <AreaRailButton
              key={area.id}
              active={isCurrent}
              label={title}
              onClick={() => {
                setFlyout(null);
                onSelectArea(area.id);
              }}
              onMouseEnter={(e) => scheduleFlyout(area, e.currentTarget)}
              onMouseLeave={scheduleClose}
            >
              <area.Icon className="h-[18px] w-[18px] shrink-0" strokeWidth={1.75} />
            </AreaRailButton>
          );
        })}
      </nav>

      {/* Footer rail: dynamic footer items (e.g. Help), Settings, Account, Collapse toggle */}
      <div className="mt-auto flex w-full shrink-0 flex-col items-center gap-1 border-t border-sidebar-border/80 px-1.5 pt-2">
        {footerItems.map((it) => {
          const label = translateNavItemLabel(t, it.id, it.label);
          const active = isNavItemActive(it.path, currentPath) || (it.id === "help" && helpActive);
          const isHelp = it.id === "help" || it.path.startsWith("/help");
          return (
            <NavLink
              key={it.id}
              to={it.path}
              onClick={() => {
                setFlyout(null);
              }}
              onMouseEnter={(e) => isHelp && scheduleFlyout(helpArea, e.currentTarget)}
              onMouseLeave={() => isHelp && scheduleClose()}
              aria-label={label}
              title={label}
              className={cn(
                "relative flex h-9 w-9 items-center justify-center rounded-md transition-colors duration-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring pointer-coarse:h-11 pointer-coarse:w-11",
                active
                  ? "bg-sidebar-accent text-accent-fg"
                  : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
              )}
            >
              {active && (
                <span
                  aria-hidden="true"
                  className="absolute -left-[7px] top-1/2 h-4 w-[3px] -translate-y-1/2 rounded-r-full bg-primary"
                />
              )}
              {it.iconStr ? (
                <span className="text-sm">{it.iconStr}</span>
              ) : (
                <it.Icon className="h-[18px] w-[18px] shrink-0" strokeWidth={1.75} />
              )}
            </NavLink>
          );
        })}

        <AreaRailButton
          active={settingsActive}
          label={t("topbar.settings")}
          onClick={() => {
            setFlyout(null);
            onOpenSettings();
          }}
          onMouseEnter={(e) => scheduleFlyout(settingsArea, e.currentTarget)}
          onMouseLeave={scheduleClose}
        >
          <Settings className="h-[18px] w-[18px] shrink-0" strokeWidth={1.75} />
        </AreaRailButton>

        <AccountMenu user={user} onNavigate={onNavigate} onLogout={onLogout} />

        <button
          type="button"
          onClick={() => {
            setFlyout(null);
            onToggle();
          }}
          aria-label={t("app.collapseMenu")}
          aria-pressed={collapsed}
          title={t("app.collapseMenu")}
          className="flex h-9 w-9 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors duration-100 hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring pointer-coarse:h-11 pointer-coarse:w-11"
        >
          {collapsed ? (
            <PanelLeft className="h-4 w-4" strokeWidth={1.75} />
          ) : (
            <PanelLeftClose className="h-4 w-4" strokeWidth={1.75} />
          )}
        </button>
      </div>

      {/* Collapsed hover flyout showing the hovered Area's grouped navigation */}
      {collapsed &&
        flyout &&
        typeof document !== "undefined" &&
        createPortal(
          <div
            data-slot="area-rail-flyout"
            role="region"
            aria-label={translateAreaTitle(t, flyout.area.id, flyout.area.title)}
            onMouseEnter={cancelClose}
            onMouseLeave={scheduleClose}
            style={
              flyout.bottom !== undefined
                ? {
                    bottom: `${flyout.bottom}px`,
                    maxHeight: `calc(100dvh - ${flyout.bottom + 12}px)`,
                  }
                : {
                    top: `${flyout.top ?? 12}px`,
                    maxHeight: `calc(100dvh - ${(flyout.top ?? 12) + 12}px)`,
                  }
            }
            className="fixed left-[56px] z-40 w-56 overflow-y-auto rounded-lg border border-border bg-popover p-2 text-popover-foreground shadow-lg"
          >
            <div className="mb-1 border-b border-border px-2.5 pb-1.5 pt-1 text-xs font-semibold text-foreground">
              {translateAreaTitle(t, flyout.area.id, flyout.area.title)}
            </div>
            <SidebarGroupList
              groups={flyout.area.groups}
              currentPath={currentPath}
              onNavigate={() => {
                setFlyout(null);
                onNavigate?.();
              }}
            />
          </div>,
          document.body,
        )}
    </div>
  );
}

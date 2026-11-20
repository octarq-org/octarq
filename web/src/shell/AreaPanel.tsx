import { NavLink } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import { Org } from "../api";
import { useTranslation } from "../i18n";
import { Area, AreaId, NavItem } from "./areas";
import { AreaRail } from "./AreaRail";
import { translateAreaTitle } from "./navI18n";
import { SidebarGroupList } from "./SidebarNav";

export function AreaPanel({
  areas,
  activeArea,
  currentArea,
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
  currentArea: Area;
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
  const { t } = useTranslation();

  return (
    <div className="flex h-full w-full bg-sidebar text-sidebar-foreground">
      <AreaRail
        areas={areas}
        activeArea={activeArea}
        settingsArea={settingsArea}
        helpArea={helpArea}
        settingsActive={settingsActive}
        helpActive={helpActive}
        user={user}
        currentPath={currentPath}
        collapsed={collapsed}
        onToggle={onToggle}
        onSelectArea={onSelectArea}
        onNavigate={onNavigate}
        onOpenSettings={onOpenSettings}
        onLogout={onLogout}
        footerItems={footerItems}
        showWorkspaceSwitcher={showWorkspaceSwitcher}
        orgs={orgs}
        activeOrgId={activeOrgId}
        activeOrgName={activeOrgName}
        onSwitchOrg={onSwitchOrg}
        onCreateOrg={onCreateOrg}
      />

      {!collapsed && (
        <div data-slot="area-subpanel" className="flex h-full w-[200px] shrink-0 flex-col bg-sidebar">
          <div className="flex h-14 shrink-0 items-center justify-between gap-2 px-3">
            <span className="truncate text-[13px] font-semibold tracking-tight text-foreground">
              {translateAreaTitle(t, currentArea.id, currentArea.title)}
            </span>
            {(settingsActive || helpActive) && (
              <NavLink
                to="/overview"
                onClick={onNavigate}
                aria-label={t("topbar.backToWorkspace", "Back to workspace")}
                title={t("topbar.backToWorkspace", "Back to workspace")}
                className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors duration-100 hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground"
              >
                <ArrowLeft className="h-3.5 w-3.5" strokeWidth={1.75} />
              </NavLink>
            )}
          </div>

          <div className="flex-1 overflow-y-auto px-2 pb-3 [scrollbar-gutter:stable]">
            <SidebarGroupList
              groups={currentArea.groups}
              currentPath={currentPath}
              onNavigate={onNavigate}
            />
          </div>
        </div>
      )}
    </div>
  );
}

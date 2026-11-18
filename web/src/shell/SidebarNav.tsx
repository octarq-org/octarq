import { NavLink } from "react-router-dom";
import { ExternalLink } from "lucide-react";
import { cn } from "@octarq/plugin-sdk";
import { useTranslation } from "../i18n";
import { NavGroup, NavItem } from "./areas";
import { isNavItemActive } from "./navActive";
import { translateGroupLabel, translateNavItemLabel } from "./navI18n";

export function SidebarNavRow({
  item,
  currentPath,
  onNavigate,
}: {
  item: NavItem;
  currentPath: string;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  const active = isNavItemActive(item.path, currentPath);
  const label = translateNavItemLabel(t, item.id, item.label);

  if (item.external) {
    return (
      <a
        href={item.path}
        onClick={onNavigate}
        className="group flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-[13px] text-muted-foreground transition-colors duration-100 hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground pointer-coarse:min-h-11"
      >
        <ExternalLink
          className="h-4 w-4 shrink-0 text-muted-foreground group-hover:text-sidebar-accent-foreground"
          strokeWidth={1.75}
        />
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
        <span className="text-[11px] font-medium text-muted-foreground">{item.badge}</span>
      )}
    </NavLink>
  );
}

export function SidebarGroupList({
  groups,
  currentPath,
  onNavigate,
}: {
  groups: NavGroup[];
  currentPath: string;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {groups.map((group) => (
        <div key={group.label} className="mb-3 last:mb-0">
          <p className="px-2.5 pb-1 pt-2 text-[11px] font-medium text-muted-foreground/80">
            {translateGroupLabel(t, group.label)}
          </p>
          <div className="space-y-0.5">
            {group.items.map((item) => (
              <SidebarNavRow
                key={item.id}
                item={item}
                currentPath={currentPath}
                onNavigate={onNavigate}
              />
            ))}
          </div>
        </div>
      ))}
    </>
  );
}

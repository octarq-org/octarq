import { cn } from "@octarq/plugin-sdk";
import { NavLink } from "react-router-dom";
import { motion } from "framer-motion";
import { Menu } from "@base-ui/react/menu";
import {
  ChevronsUpDown, CheckIcon, Plus, PanelLeftClose, PanelLeft,
  HelpCircle, BookOpen, Info, MessageCircle, ExternalLink,
} from "lucide-react";
import { Org } from "../api";
import { useTranslation } from "../i18n";
import { Area, NavItem } from "./areas";
import {
  translateAreaTitle,
  translateAreaSubtitle,
  translateGroupLabel,
  translateNavItemLabel,
} from "./navI18n";
import { RESOURCES } from "./resources";
import { MENU_ITEM, MENU_POPUP } from "./menuStyles";
import { isNavItemActive } from "./navActive";

// lucide-react @1.x dropped the `Github` glyph, so the mark is inlined (same
// path the Login page uses for the GitHub OAuth button).
function GithubIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
    </svg>
  );
}

// AreaPanel is the left navigation rail. It owns three stacked regions —
// a header (workspace switcher on Pro, area title otherwise), the grouped
// second-level nav for the active area, and a footer with the collapse toggle.
// When `collapsed` it renders as a 64px icon rail (icons + tooltips) instead of
// disappearing, so navigation stays reachable. The collapse animation and the
// rail width live in the shell (App.tsx); this stays presentational.
export function AreaPanel({
  area,
  currentPath,
  collapsed,
  onToggle,
  onNavigate,
  footerItems,
  showWorkspaceSwitcher,
  orgs,
  activeOrgId,
  activeOrgName,
  onSwitchOrg,
  onCreateOrg,
}: {
  area: Area;
  currentPath: string;
  collapsed: boolean;
  onToggle: () => void;
  // Fired when a nav item is chosen — the shell uses it to close the mobile
  // drawer after navigation.
  onNavigate?: () => void;
  // Plugin-contributed footer placement items (category "footer") — e.g. the
  // Pro Help plugin — listed above the static octarq resources.
  footerItems: NavItem[];
  showWorkspaceSwitcher: boolean;
  orgs: Org[];
  activeOrgId: number;
  activeOrgName: string;
  onSwitchOrg: (id: number) => void;
  onCreateOrg: () => void;
}) {
  const { t } = useTranslation();

  const orgInitials = activeOrgName
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();

  return (
    <div className="flex h-full w-full flex-col border-r border-border bg-background/70 backdrop-blur-xl">
      {/* ── Header: workspace switcher (Pro) or area title ── */}
      <div className={cn("border-b border-border", collapsed ? "p-2" : "px-4 py-4")}>
        {showWorkspaceSwitcher ? (
          <Menu.Root>
            <Menu.Trigger
              aria-label={t("topbar.switchWorkspace")}
              title={collapsed ? activeOrgName : undefined}
              className={cn(
                "tap-expand flex cursor-pointer items-center rounded-[var(--radius)] border border-border bg-card shadow-[0_1px_1px_rgb(0_0_0/0.03)] transition-colors duration-[var(--dur-fast)] hover:border-border-strong data-[popup-open]:border-border-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-border",
                collapsed ? "h-10 w-10 justify-center" : "h-11 w-full gap-2.5 px-2",
              )}
            >
              <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-accent-soft text-[11px] font-semibold text-accent-fg">
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
                  <div className="px-2 py-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{t("topbar.workspaces")}</div>
                  {orgs.map((o) => (
                    <Menu.Item key={o.id} onClick={() => onSwitchOrg(o.id)} className={MENU_ITEM}>
                      <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-accent-soft text-[11px] font-semibold text-accent-fg ring-1 ring-inset ring-border">
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
        ) : collapsed ? (
          <div
            title={t(`areas.${area.id}.title`, area.title)}
            className="flex h-10 w-10 items-center justify-center rounded-[var(--radius)] bg-well text-foreground"
          >
            <area.Icon className="h-[18px] w-[18px]" strokeWidth={1.75} aria-hidden="true" />
          </div>
        ) : (
          <div className="px-1">
            <h2 className="truncate font-display text-[1.0625rem] font-semibold leading-tight tracking-[-0.02em] text-foreground">
              {translateAreaTitle(t, area.id, area.title)}
            </h2>
            <p className="mt-0.5 truncate text-[11.5px] leading-snug text-muted-foreground">
              {translateAreaSubtitle(t, area.id, area.subtitle)}
            </p>
          </div>
        )}
      </div>

      {/* ── Grouped nav ── */}
      <div className={cn("flex-1 overflow-y-auto py-3 [scrollbar-gutter:stable]", collapsed ? "px-2" : "px-3")}>
        {area.groups.map((group, gi) => (
          <div key={group.label} className={collapsed ? "mb-1" : "mb-4"}>
            {collapsed
              ? gi > 0 && <div className="mx-auto my-1.5 h-px w-6 bg-border" />
              : (
                <p className="flex items-center gap-2 px-2.5 pb-2 pt-3 font-mono text-[10px] font-medium uppercase tracking-[0.1em] text-muted-foreground/70">
                  <span className="inline-block h-1 w-1 rounded-[1px] bg-accent-fg/70" />
                  {translateGroupLabel(t, group.label)}
                </p>
              )}
            <div className="space-y-0.5">
              {group.items.map((item) => {
                const active = isNavItemActive(item.path, currentPath);
                if (collapsed) {
                  if (item.external) {
                    return (
                      <a
                        key={item.id}
                        href={item.path}
                        onClick={onNavigate}
                        title={translateNavItemLabel(t, item.id, item.label)}
                        aria-label={translateNavItemLabel(t, item.id, item.label)}
                        className="relative mx-auto flex h-10 w-10 items-center justify-center rounded-[var(--radius)] text-muted-foreground transition-colors pointer-coarse:h-11 pointer-coarse:w-11 hover:bg-surface-hover hover:text-foreground"
                      >
                        <ExternalLink className="h-[18px] w-[18px]" strokeWidth={1.75} />
                      </a>
                    );
                  }
                  return (
                    <NavLink
                      key={item.id}
                      to={item.path}
                      onClick={onNavigate}
                      title={translateNavItemLabel(t, item.id, item.label)}
                      aria-label={translateNavItemLabel(t, item.id, item.label)}
                      className={cn(
                        "relative mx-auto flex h-10 w-10 items-center justify-center rounded-[var(--radius)] transition-colors pointer-coarse:h-11 pointer-coarse:w-11",
                        active
                          ? "bg-accent-soft text-accent-fg ring-1 ring-inset ring-accent-border"
                          : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
                      )}
                    >
                      {item.iconStr ? (
                        <span className="text-sm">{item.iconStr}</span>
                      ) : (
                        <item.Icon className="h-[18px] w-[18px]" strokeWidth={1.75} />
                      )}
                      {item.badge !== undefined && (
                        <span className="absolute -right-0.5 -top-0.5 flex h-2 w-2 rounded-full bg-accent-fg" />
                      )}
                    </NavLink>
                  );
                }
                if (item.external) {
                  return (
                    <a
                      key={item.id}
                      href={item.path}
                      onClick={onNavigate}
                      className="group relative flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left text-[13px] text-muted-foreground transition-colors pointer-coarse:min-h-11 duration-[var(--dur-fast)] hover:bg-surface-hover hover:text-foreground"
                    >
                      <ExternalLink className="relative h-[18px] w-[18px] text-muted-foreground group-hover:text-foreground" strokeWidth={1.75} />
                      <span className="relative flex-1 truncate">{translateNavItemLabel(t, item.id, item.label)}</span>
                    </a>
                  );
                }
                return (
                  <NavLink
                    key={item.id}
                    to={item.path}
                    onClick={onNavigate}
                    className={`group relative flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left text-[13px] transition-colors pointer-coarse:min-h-11 duration-[var(--dur-fast)] ${
                      active ? "font-medium text-foreground" : "text-muted-foreground hover:bg-surface-hover hover:text-foreground"
                    }`}
                  >
                    {active && (
                      <motion.span
                        layoutId="panel-active"
                        transition={{ type: "spring", stiffness: 500, damping: 40 }}
                        className="absolute inset-0 rounded-lg bg-accent-soft ring-1 ring-inset ring-accent-border"
                      >
                        <span className="absolute left-0 top-1/2 h-4 w-[2px] -translate-y-1/2 rounded-full bg-accent-fg" />
                      </motion.span>
                    )}
                    {item.iconStr ? (
                      <span className={`relative text-sm ${active ? "text-accent-fg" : ""}`}>
                        {item.iconStr}
                      </span>
                    ) : (
                      <item.Icon
                        className={`relative h-[18px] w-[18px] ${active ? "text-accent-fg" : "text-muted-foreground group-hover:text-foreground"}`}
                        strokeWidth={1.75}
                      />
                    )}
                    <span className="relative flex-1 truncate">{translateNavItemLabel(t, item.id, item.label)}</span>
                    {item.badge !== undefined && (
                      <span className="relative text-[11px] font-medium text-muted-foreground">
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

      {/* ── Footer: plugin footer items + octarq's own links + collapse toggle ──
          Kept always-available and strictly apart from the org's business nav
          above. Footer-placed plugin menus (category "footer") render as rail
          links here; the menu below holds only octarq's external links.

          Help is one of those plugin menus, not a hardcoded route. It used to be
          both — a literal <NavLink to="/help"> here AND the help plugin's own
          Menus() entry, which lands in footerItems — so it rendered twice, once
          as a rail link and once inside the menu. Hardcoding it also broke the
          repo rule that the Go half is the only source of sidebar placement
          (website/src/content/docs/writing-a-plugin.md): a build without the
          help plugin still showed the
          link, pointing at a route nothing served. */}
      <div className={cn("border-t border-border", collapsed ? "space-y-1 p-2" : "space-y-0.5 px-3 py-2")}>
        {footerItems.map((it) => {
          const label = translateNavItemLabel(t, it.id, it.label);
          const active = isNavItemActive(it.path, currentPath);
          return (
            <NavLink
              key={it.id}
              to={it.path}
              onClick={onNavigate}
              aria-label={label}
              title={collapsed ? label : undefined}
              className={cn(
                "flex items-center rounded-xl transition-colors",
                active
                  ? "bg-accent-soft font-bold text-accent-fg shadow-2xs"
                  : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
                collapsed ? "mx-auto h-10 w-10 justify-center pointer-coarse:h-11 pointer-coarse:w-11" : "h-9 w-full gap-2 px-2.5 text-[13px] font-medium pointer-coarse:min-h-11",
              )}
            >
              {it.iconStr ? (
                <span className="w-[18px] shrink-0 text-center text-sm">{it.iconStr}</span>
              ) : (
                <it.Icon className={cn("h-[18px] w-[18px] shrink-0", active && "text-accent-fg")} strokeWidth={1.75} />
              )}
              {!collapsed && <span className="truncate">{label}</span>}
            </NavLink>
          );
        })}

        <Menu.Root>
          <Menu.Trigger
            aria-label={t("footer.help", "About octarq")}
            title={collapsed ? t("footer.help", "About octarq") : undefined}
            className={cn(
              "flex cursor-pointer items-center rounded-lg text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground data-[popup-open]:bg-surface-hover data-[popup-open]:text-foreground",
              collapsed ? "mx-auto h-10 w-10 justify-center pointer-coarse:h-11 pointer-coarse:w-11" : "h-9 w-full gap-2 px-2.5 text-[13px] font-medium pointer-coarse:min-h-11",
            )}
          >
            <HelpCircle className="h-[18px] w-[18px]" strokeWidth={1.75} />
            {!collapsed && <span>{t("footer.help", "About octarq")}</span>}
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Positioner side={collapsed ? "right" : "top"} align="start" sideOffset={8} className="z-50 outline-none">
              <Menu.Popup className={cn(MENU_POPUP, "w-60")}>
                {/* Only octarq's own external links. Plugin footer items render
                    as rail links above — listing them here too is what put Help
                    in the sidebar twice. */}
                <Menu.Item render={<a href={RESOURCES.docs} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
                  <BookOpen className="h-4 w-4" strokeWidth={1.75} />
                  <span className="flex-1">{t("footer.docs", "Developer Docs")}</span>
                  <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </Menu.Item>
                <Menu.Item render={<a href={RESOURCES.about} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
                  <Info className="h-4 w-4" strokeWidth={1.75} />
                  <span className="flex-1">{t("footer.about", "Website")}</span>
                  <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </Menu.Item>
                <Menu.Item render={<a href={RESOURCES.github} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
                  <GithubIcon className="h-4 w-4" />
                  <span className="flex-1">{t("footer.github", "GitHub")}</span>
                  <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </Menu.Item>
                <Menu.Item render={<a href={RESOURCES.contact} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
                  <MessageCircle className="h-4 w-4" strokeWidth={1.75} />
                  <span className="flex-1">{t("footer.contact", "Contact")}</span>
                  <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </Menu.Item>
              </Menu.Popup>
            </Menu.Positioner>
          </Menu.Portal>
        </Menu.Root>

        <button
          onClick={onToggle}
          aria-label={t("app.collapseMenu")}
          aria-pressed={collapsed}
          title={t("app.collapseMenu")}
          className={cn(
            "flex cursor-pointer items-center rounded-lg text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground",
            collapsed ? "mx-auto h-10 w-10 justify-center" : "h-9 w-full gap-2 px-2.5 text-[13px] font-medium",
          )}
        >
          {collapsed ? (
            <PanelLeft className="h-[18px] w-[18px]" strokeWidth={1.75} />
          ) : (
            <>
              <PanelLeftClose className="h-[18px] w-[18px]" strokeWidth={1.75} />
              <span>{t("app.collapseMenu")}</span>
            </>
          )}
        </button>
      </div>
    </div>
  );
}

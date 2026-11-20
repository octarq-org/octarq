import type { UIArea } from "@octarq/plugin-sdk";
import { Globe, LayoutDashboard, LucideIcon, Workflow, Boxes, Settings } from "lucide-react";
import { Action, MenuItem, PluginInfo } from "../api";
import { Area, AreaId, NavGroup, NavItem, STATIC_AREAS, SETTINGS_AREA, FOOTER_PLACEMENT, menuIcon, pluginAreaToArea } from "./areas";
import { roleSatisfies } from "./role";

export const BUILTIN_AREA_GROUPS: Record<string, string[]> = {
  operations: ["Workspace", "Marketing", "Messaging", "Security", "System"],
  assets: ["Network", "Hosting", "Storage & Databases"],
  settings: ["Instance", "Workspace", "Integrations", "Account"],
};

export interface BuildNavigationTreeOptions {
  menus?: MenuItem[];
  plugins?: PluginInfo[];
  pluginAreas?: UIArea[];
  role?: string;
  isInstanceAdmin?: boolean;
  backendLoaded?: boolean;
  helpArea?: Area;
  showPersonalArea?: boolean;
}

export class NavigationTree {
  readonly areas: Area[];
  readonly settingsArea: Area;
  readonly footerItems: NavItem[];
  readonly helpArea?: Area;
  readonly allAreas: Area[];
  readonly settingsPaths: Set<string>;
  readonly disabledPlugins: Set<string>;
  readonly disabledPaths: Set<string>;

  constructor({
    areas,
    settingsArea,
    footerItems,
    helpArea,
    settingsPaths,
    disabledPlugins,
    disabledPaths,
  }: {
    areas: Area[];
    settingsArea: Area;
    footerItems: NavItem[];
    helpArea?: Area;
    settingsPaths: Set<string>;
    disabledPlugins: Set<string>;
    disabledPaths: Set<string>;
  }) {
    this.areas = areas;
    this.settingsArea = settingsArea;
    this.footerItems = footerItems;
    this.helpArea = helpArea;
    this.settingsPaths = settingsPaths;
    this.disabledPlugins = disabledPlugins;
    this.disabledPaths = disabledPaths;
    this.allAreas = [...areas, settingsArea, ...(helpArea ? [helpArea] : [])];
  }

  static resolveCategoryToArea(cat?: string, pluginAreas: UIArea[] = []): AreaId {
    const c = (cat ?? "").trim().toLowerCase();
    if (!c) return "operations";

    if (c === FOOTER_PLACEMENT || c === "resources") return FOOTER_PLACEMENT;
    if (c === "settings" || c === "instance" || c === "account" || c === "integrations") return "settings";
    if (c === "personal") return "personal";

    const pluginHit = pluginAreas.find(
      (a) =>
        a.id.toLowerCase() === c ||
        (a.groups ?? []).some((g) => g.toLowerCase() === c),
    );
    if (pluginHit) return pluginHit.id;

    if (
      c === "assets" ||
      c === "infrastructure" ||
      BUILTIN_AREA_GROUPS.assets.some((g) => g.toLowerCase() === c)
    ) {
      return "assets";
    }

    if (
      c === "operations" ||
      BUILTIN_AREA_GROUPS.operations.some((g) => g.toLowerCase() === c)
    ) {
      return "operations";
    }

    return "operations";
  }

  static resolveMenuToArea(menu: MenuItem, pluginAreas: UIArea[] = []): AreaId {
    const rawCat = (menu.category ?? "").trim().toLowerCase();
    if (rawCat === FOOTER_PLACEMENT || rawCat === "resources") {
      return FOOTER_PLACEMENT;
    }

    const rawArea = (menu.area ?? "").trim().toLowerCase();
    if (rawArea) {
      if (rawArea === FOOTER_PLACEMENT || rawArea === "resources") return FOOTER_PLACEMENT;
      if (rawArea === "settings" || rawArea === "instance" || rawArea === "account" || rawArea === "integrations") return "settings";
      if (rawArea === "assets" || rawArea === "infrastructure") return "assets";
      if (rawArea === "operations") return "operations";

      const pluginHit = pluginAreas.find((a) => a.id.toLowerCase() === rawArea);
      if (pluginHit) return pluginHit.id;

      return rawArea;
    }

    return NavigationTree.resolveCategoryToArea(menu.category, pluginAreas);
  }

  static build(options: BuildNavigationTreeOptions = {}): NavigationTree {
    const menus = options.menus ?? [];
    const plugins = options.plugins ?? [];
    const role = options.role;
    const isInstanceAdmin = !!options.isInstanceAdmin;
    const helpArea = options.helpArea;
    const showPersonalArea = options.showPersonalArea ?? true;

    const disabledPlugins = new Set(plugins.filter((p) => !p.enabled).map((p) => p.key));
    const disabledPaths = new Set(
      plugins.filter((p) => !p.enabled).flatMap((p) => p.menus.map((m) => m.path)),
    );

    const seenPaths = new Set<string>();
    const dedupedMenus = menus.filter((m) => {
      if (seenPaths.has(m.path)) return false;
      seenPaths.add(m.path);
      return true;
    });

    const validPluginAreas = (options.pluginAreas ?? []).filter(
      (pa) => pa.id !== "settings" && !STATIC_AREAS.some((sa) => sa.id === pa.id),
    );

    const baseAreas = [
      ...STATIC_AREAS,
      SETTINGS_AREA,
      ...validPluginAreas.map(pluginAreaToArea),
    ];

    const staticPaths = new Set(
      baseAreas.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => i.path))),
    );

    const extras = dedupedMenus.filter(
      (m) =>
        !staticPaths.has(m.path) &&
        !disabledPaths.has(m.path) &&
        (showPersonalArea ||
          NavigationTree.resolveMenuToArea(m, validPluginAreas) !== "personal") &&
        roleSatisfies(m.requiredRole, role, isInstanceAdmin),
    );

    const toNavItem = (m: MenuItem): NavItem & { order: number } => {
      const KeyIcon = menuIcon(m.icon);
      return {
        id: m.id,
        label: m.label,
        Icon: KeyIcon ?? Globe,
        iconStr: KeyIcon ? undefined : m.icon,
        path: m.path,
        order: m.order ?? 0,
        external: m.external || undefined,
      };
    };

    const footerItems = extras
      .filter((m) => NavigationTree.resolveMenuToArea(m, validPluginAreas) === FOOTER_PLACEMENT)
      .map(toNavItem)
      .sort((a, b) => a.order - b.order);

    const mergedAreas = baseAreas.map((baseArea) => {
      const groups: NavGroup[] = baseArea.groups.map((g) => ({
        label: g.label,
        items: g.items.filter((i) => !disabledPaths.has(i.path)),
      }));

      const areaExtras = extras.filter(
        (m) => NavigationTree.resolveMenuToArea(m, validPluginAreas) === baseArea.id,
      );

      areaExtras.forEach((m) => {
        const item = toNavItem(m);

        const effectiveCategory =
          baseArea.id === "settings" && (m.category || "").toLowerCase() === "settings"
            ? "Workspace"
            : (m.category || "");

        const matchedGroup = groups.find(
          (g) => g.label.toLowerCase() === effectiveCategory.toLowerCase(),
        );

        if (matchedGroup) {
          matchedGroup.items.push(item);
        } else {
          const groupName = effectiveCategory || "More";
          const dynamicGroup = groups.find((g) => g.label === groupName);
          if (dynamicGroup) {
            dynamicGroup.items.push(item);
          } else {
            groups.push({
              label: groupName,
              items: [item],
            });
          }
        }
      });

      groups.forEach((g) => {
        g.items.sort((a: any, b: any) => (a.order ?? 0) - (b.order ?? 0));
      });

      const declaredOrder =
        BUILTIN_AREA_GROUPS[baseArea.id] ??
        validPluginAreas.find((pa) => pa.id === baseArea.id)?.groups ??
        [];

      if (declaredOrder.length > 0) {
        groups.sort((a, b) => {
          const idxA = declaredOrder.findIndex((g) => g.toLowerCase() === a.label.toLowerCase());
          const idxB = declaredOrder.findIndex((g) => g.toLowerCase() === b.label.toLowerCase());
          if (idxA !== -1 && idxB !== -1) return idxA - idxB;
          if (idxA !== -1) return -1;
          if (idxB !== -1) return 1;
          return 0;
        });
      }

      return {
        ...baseArea,
        groups: groups.filter((g) => g.items.length > 0),
      };
    });

    const rawSettings = mergedAreas.find((a) => a.id === "settings") ?? SETTINGS_AREA;
    const settingsArea: Area = isInstanceAdmin
      ? rawSettings
      : {
          ...rawSettings,
          groups: rawSettings.groups.filter((g) => g.label !== "Instance"),
        };

    const settingsPaths = new Set(
      settingsArea.groups.flatMap((g) => g.items.map((i) => i.path)),
    );

    const businessAreas = mergedAreas.filter(
      (a) =>
        a.id !== "settings" &&
        (showPersonalArea || a.id !== "personal") &&
        a.groups.length > 0,
    );

    return new NavigationTree({
      areas: businessAreas,
      settingsArea,
      footerItems,
      helpArea,
      settingsPaths,
      disabledPlugins,
      disabledPaths,
    });
  }

  findArea(id: AreaId): Area | undefined {
    return this.allAreas.find((a) => a.id === id);
  }

  isSettingsPath(path: string): boolean {
    if (path.startsWith("/settings")) return true;
    return [...this.settingsPaths].some(
      (p) => path === p || path.startsWith(p + "/"),
    );
  }

  isHelpPath(path: string): boolean {
    return path.startsWith("/help") || path.startsWith("/admin/help");
  }

  areaForPath(path: string): AreaId {
    if (this.isSettingsPath(path)) return "settings";
    if (this.isHelpPath(path)) return "help";

    const hit = this.areas
      .flatMap((a) =>
        a.groups.flatMap((g) =>
          g.items.map((i) => ({ prefix: i.path, area: a.id })),
        ),
      )
      .sort((x, y) => y.prefix.length - x.prefix.length)
      .find(({ prefix }) => path === prefix || path.startsWith(prefix + "/"));

    return hit?.area ?? "operations";
  }

  resolveActiveArea(pathname: string): AreaId {
    if (this.isHelpPath(pathname)) return "help";
    if (this.isSettingsPath(pathname)) return "settings";
    return this.areaForPath(pathname);
  }

  getCurrentArea(activeAreaId: AreaId): Area {
    if (activeAreaId === "help" && this.helpArea) return this.helpArea;
    if (activeAreaId === "settings") return this.settingsArea;
    return this.findArea(activeAreaId) ?? this.areas[0] ?? STATIC_AREAS[0];
  }
}

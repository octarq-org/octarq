import type { UIArea } from "@octarq/plugin-sdk";
import { Globe, LayoutDashboard, LucideIcon, Workflow, Boxes, Settings } from "lucide-react";
import { Action, MenuItem, PluginInfo } from "../api";
import { Area, AreaId, NavGroup, NavItem, STATIC_AREAS, SETTINGS_AREA, FOOTER_PLACEMENT, menuIcon, pluginAreaToArea } from "./areas";
import { roleSatisfies } from "./role";

export const BUILTIN_AREA_GROUPS: Record<string, string[]> = {
  operations: ["Workspace", "Marketing", "Messaging", "Security", "System"],
  assets: ["Network", "Hosting", "Storage & Databases"],
  settings: ["Instance", "Workspace", "Personal"],
};

export interface BuildNavigationTreeOptions {
  menus?: MenuItem[];
  plugins?: PluginInfo[];
  pluginAreas?: UIArea[];
  role?: string;
  isInstanceAdmin?: boolean;
  backendLoaded?: boolean;
  helpArea?: Area;
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

  /**
   * Deterministically resolves a category string to an AreaId or FOOTER_PLACEMENT.
   * Eliminates keyword substring heuristic matching (c.includes(...)) in favor of
   * exact matching against area IDs and declared group labels.
   */
  static resolveCategoryToArea(cat?: string, pluginAreas: UIArea[] = []): AreaId {
    const c = (cat ?? "").trim().toLowerCase();
    if (!c) return "operations";

    // Special placement keywords
    if (c === FOOTER_PLACEMENT || c === "resources") return FOOTER_PLACEMENT;
    if (c === "settings" || c === "instance" || c === "account" || c === "personal") return "settings";

    // Match plugin-declared areas by ID or declared group labels (never title)
    const pluginHit = pluginAreas.find(
      (a) =>
        a.id.toLowerCase() === c ||
        (a.groups ?? []).some((g) => g.toLowerCase() === c),
    );
    if (pluginHit) return pluginHit.id;

    // Match built-in Infrastructure/Assets area
    if (
      c === "assets" ||
      c === "infrastructure" ||
      BUILTIN_AREA_GROUPS.assets.some((g) => g.toLowerCase() === c)
    ) {
      return "assets";
    }

    // Match built-in Operations area
    if (
      c === "operations" ||
      BUILTIN_AREA_GROUPS.operations.some((g) => g.toLowerCase() === c)
    ) {
      return "operations";
    }

    // Default fallback for unclaimed categories
    return "operations";
  }

  /**
   * Deterministically resolves a menu item to an AreaId or FOOTER_PLACEMENT.
   * Prioritizes explicit `menu.area` metadata declared by the plugin/backend
   * before falling back to category-based area resolution.
   */
  static resolveMenuToArea(menu: MenuItem, pluginAreas: UIArea[] = []): AreaId {
    const rawCat = (menu.category ?? "").trim().toLowerCase();
    if (rawCat === FOOTER_PLACEMENT || rawCat === "resources") {
      return FOOTER_PLACEMENT;
    }

    const rawArea = (menu.area ?? "").trim().toLowerCase();
    if (rawArea) {
      if (rawArea === FOOTER_PLACEMENT || rawArea === "resources") return FOOTER_PLACEMENT;
      if (rawArea === "settings" || rawArea === "instance" || rawArea === "account" || rawArea === "personal") return "settings";
      if (rawArea === "assets" || rawArea === "infrastructure") return "assets";
      if (rawArea === "operations") return "operations";

      // Match plugin-declared areas by ID
      const pluginHit = pluginAreas.find((a) => a.id.toLowerCase() === rawArea);
      if (pluginHit) return pluginHit.id;

      // If declared area is any custom area id, respect it
      return rawArea;
    }

    return NavigationTree.resolveCategoryToArea(menu.category, pluginAreas);
  }

  /**
   * Pure logic builder: merges backend menus, plugin areas, role gating, and disabled states
   * into a high-cohesion NavigationTree model.
   */
  static build(options: BuildNavigationTreeOptions = {}): NavigationTree {
    const menus = options.menus ?? [];
    const plugins = options.plugins ?? [];
    const role = options.role;
    const isInstanceAdmin = !!options.isInstanceAdmin;
    const helpArea = options.helpArea;

    const disabledPlugins = new Set(plugins.filter((p) => !p.enabled).map((p) => p.key));
    const disabledPaths = new Set(
      plugins.filter((p) => !p.enabled).flatMap((p) => p.menus.map((m) => m.path)),
    );

    // Deduplicate backend menus by path
    const seenPaths = new Set<string>();
    const dedupedMenus = menus.filter((m) => {
      if (seenPaths.has(m.path)) return false;
      seenPaths.add(m.path);
      return true;
    });

    // Extract valid plugin areas (avoiding "settings" and collisions with static areas)
    const validPluginAreas = (options.pluginAreas ?? []).filter(
      (pa) => pa.id !== "settings" && !STATIC_AREAS.some((sa) => sa.id === pa.id),
    );

    // Base area list: built-in static areas, settings area, and plugin areas
    const baseAreas = [
      ...STATIC_AREAS,
      SETTINGS_AREA,
      ...validPluginAreas.map(pluginAreaToArea),
    ];

    const staticPaths = new Set(
      baseAreas.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => i.path))),
    );

    // Filter dynamic items by static presence, disabled plugins, and role authorization
    const extras = dedupedMenus.filter(
      (m) =>
        !staticPaths.has(m.path) &&
        !disabledPaths.has(m.path) &&
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
      };
    };

    // Separate footer-placed items
    const footerItems = extras
      .filter((m) => NavigationTree.resolveMenuToArea(m, validPluginAreas) === FOOTER_PLACEMENT)
      .map(toNavItem)
      .sort((a, b) => a.order - b.order);

    // Assemble groups for each area
    const mergedAreas = baseAreas.map((baseArea) => {
      // Retain static items that are not owned by disabled plugins
      const groups: NavGroup[] = baseArea.groups.map((g) => ({
        label: g.label,
        items: g.items.filter((i) => !disabledPaths.has(i.path)),
      }));

      // Menus directed to this area
      const areaExtras = extras.filter(
        (m) => NavigationTree.resolveMenuToArea(m, validPluginAreas) === baseArea.id,
      );

      areaExtras.forEach((m) => {
        const item = toNavItem(m);

        // In Settings area, generic "settings" category routes to the "Workspace" group
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

      // Sort items within each group
      groups.forEach((g) => {
        g.items.sort((a: any, b: any) => (a.order ?? 0) - (b.order ?? 0));
      });

      // Enforce declared group ordering if specified
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

      // Drop groups that have no items
      return {
        ...baseArea,
        groups: groups.filter((g) => g.items.length > 0),
      };
    });

    // Resolve Settings area
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

    // Business areas (excluding settings and dropping empty areas)
    const businessAreas = mergedAreas.filter(
      (a) => a.id !== "settings" && a.groups.length > 0,
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

  /**
   * Deterministic path to AreaId resolution using longest prefix matching.
   */
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

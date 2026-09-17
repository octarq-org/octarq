import { describe, it, expect } from "vitest";
import type { UIArea } from "@octarq/plugin-sdk";
import { NavigationTree, BUILTIN_AREA_GROUPS } from "./NavigationTree";
import { MenuItem, PluginInfo } from "../api";
import { FOOTER_PLACEMENT } from "./areas";

const COMMERCE_AREA: UIArea = {
  id: "commerce",
  title: "Commerce",
  subtitle: "Revenue and billing",
  icon: "wallet",
  groups: ["Sales", "Finance"],
};

describe("NavigationTree domain model", () => {
  describe("NavigationTree.resolveCategoryToArea", () => {
    it("matches special placement keywords", () => {
      expect(NavigationTree.resolveCategoryToArea("footer")).toBe(FOOTER_PLACEMENT);
      expect(NavigationTree.resolveCategoryToArea("resources")).toBe(FOOTER_PLACEMENT);
      expect(NavigationTree.resolveCategoryToArea("settings")).toBe("settings");
      expect(NavigationTree.resolveCategoryToArea("instance")).toBe("settings");
      expect(NavigationTree.resolveCategoryToArea("account")).toBe("settings");
      expect(NavigationTree.resolveCategoryToArea("personal")).toBe("settings");
    });

    it("matches plugin-declared areas by id or declared group label", () => {
      expect(NavigationTree.resolveCategoryToArea("commerce", [COMMERCE_AREA])).toBe("commerce");
      expect(NavigationTree.resolveCategoryToArea("COMMERCE", [COMMERCE_AREA])).toBe("commerce");
      expect(NavigationTree.resolveCategoryToArea("Sales", [COMMERCE_AREA])).toBe("commerce");
      expect(NavigationTree.resolveCategoryToArea("finance", [COMMERCE_AREA])).toBe("commerce");
    });

    it("does NOT match plugin area by title", () => {
      const custom: UIArea = { ...COMMERCE_AREA, title: "Storefront" };
      expect(NavigationTree.resolveCategoryToArea("Storefront", [custom])).not.toBe("commerce");
    });

    it("matches built-in areas deterministically by id or declared group label", () => {
      expect(NavigationTree.resolveCategoryToArea("assets")).toBe("assets");
      expect(NavigationTree.resolveCategoryToArea("infrastructure")).toBe("assets");
      expect(NavigationTree.resolveCategoryToArea("Network")).toBe("assets");
      expect(NavigationTree.resolveCategoryToArea("hosting")).toBe("assets");
      expect(NavigationTree.resolveCategoryToArea("Storage & Databases")).toBe("assets");

      expect(NavigationTree.resolveCategoryToArea("operations")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("Workspace")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("Marketing")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("messaging")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("Security")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("System")).toBe("operations");
    });

    it("eliminates keyword fuzzy substring heuristics and plugin category hardcoding", () => {
      // Previously, any category containing "asset", "infra", "compute", etc. would match "assets".
      // Specific plugin words like "storage" and "databases" are no longer hardcoded in resolveCategoryToArea.
      // With deterministic exact matching, non-declared strings fall back to "operations".
      expect(NavigationTree.resolveCategoryToArea("storage")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("databases")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("asset-tracking")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("cloud-compute-hub")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("infra_nodes")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("security-audit-tool")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea("unknown-category")).toBe("operations");
      expect(NavigationTree.resolveCategoryToArea(undefined)).toBe("operations");
    });
  });

  describe("NavigationTree.resolveMenuToArea", () => {
    it("prioritizes explicit area metadata over category fallback", () => {
      expect(
        NavigationTree.resolveMenuToArea({
          id: "s3",
          label: "Storage",
          path: "/storage",
          icon: "hard-drive",
          category: "storage",
          area: "assets",
        }),
      ).toBe("assets");

      expect(
        NavigationTree.resolveMenuToArea({
          id: "pg",
          label: "Databases",
          path: "/databases",
          icon: "database",
          category: "databases",
          area: "assets",
        }),
      ).toBe("assets");

      expect(
        NavigationTree.resolveMenuToArea({
          id: "custom",
          label: "Custom Tool",
          path: "/custom",
          icon: "puzzle",
          category: "operations",
          area: "assets",
        }),
      ).toBe("assets");

      expect(
        NavigationTree.resolveMenuToArea({
          id: "links",
          label: "Links",
          path: "/links",
          icon: "link-2",
          category: "Marketing",
          area: "operations",
        }),
      ).toBe("operations");

      expect(
        NavigationTree.resolveMenuToArea({
          id: "custom-settings",
          label: "Custom Settings",
          path: "/settings/custom",
          icon: "settings",
          category: "Custom",
          area: "settings",
        }),
      ).toBe("settings");
    });

    it("respects plugin-declared UIArea via area metadata", () => {
      expect(
        NavigationTree.resolveMenuToArea(
          {
            id: "billing",
            label: "Billing",
            path: "/billing",
            icon: "wallet",
            category: "billing",
            area: "commerce",
          },
          [COMMERCE_AREA],
        ),
      ).toBe("commerce");
    });

    it("falls back to category resolution when area metadata is omitted", () => {
      expect(
        NavigationTree.resolveMenuToArea({
          id: "domains",
          label: "DNS",
          path: "/domains",
          icon: "globe",
          category: "Network",
        }),
      ).toBe("assets");

      expect(
        NavigationTree.resolveMenuToArea({
          id: "overview",
          label: "Overview",
          path: "/overview",
          icon: "layout-dashboard",
          category: "Workspace",
        }),
      ).toBe("operations");

      expect(
        NavigationTree.resolveMenuToArea({
          id: "unclaimed",
          label: "Unclaimed",
          path: "/unclaimed",
          icon: "puzzle",
          category: "storage",
        }),
      ).toBe("operations");
    });

    it("routes footer placement accurately", () => {
      expect(
        NavigationTree.resolveMenuToArea({
          id: "help",
          label: "Help",
          path: "/help",
          icon: "book",
          category: "footer",
        }),
      ).toBe(FOOTER_PLACEMENT);

      expect(
        NavigationTree.resolveMenuToArea({
          id: "docs",
          label: "Docs",
          path: "/docs",
          icon: "book-open",
          category: "resources",
        }),
      ).toBe(FOOTER_PLACEMENT);

      expect(
        NavigationTree.resolveMenuToArea({
          id: "extra-footer",
          label: "Extra",
          path: "/extra",
          icon: "book",
          category: "documentation",
          area: "footer",
        }),
      ).toBe(FOOTER_PLACEMENT);
    });
  });

  describe("NavigationTree.build", () => {
    it("assembles menus into appropriate areas and drops empty areas", () => {
      const menus: MenuItem[] = [
        { id: "overview", label: "Overview", path: "/overview", icon: "layout-dashboard", category: "Workspace" },
        { id: "links", label: "Links", path: "/links", icon: "link-2", category: "Marketing" },
        { id: "domains", label: "DNS", path: "/domains", icon: "globe", category: "Network" },
        { id: "help", label: "Help", path: "/help", icon: "book", category: "footer" },
      ];

      const tree = NavigationTree.build({
        menus,
        plugins: [],
        pluginAreas: [],
        backendLoaded: true,
      });

      // Business areas: operations and assets both have items, so both are present
      const areaIds = tree.areas.map((a) => a.id);
      expect(areaIds).toContain("operations");
      expect(areaIds).toContain("assets");

      // Check operations groups
      const ops = tree.findArea("operations")!;
      expect(ops).toBeDefined();
      const opsGroupLabels = ops.groups.map((g) => g.label);
      expect(opsGroupLabels).toContain("Workspace");
      expect(opsGroupLabels).toContain("Marketing");

      // Check assets groups
      const assets = tree.findArea("assets")!;
      expect(assets).toBeDefined();
      const assetsGroupLabels = assets.groups.map((g) => g.label);
      expect(assetsGroupLabels).toContain("Network");

      // Footer items contains help
      expect(tree.footerItems.some((f) => f.id === "help")).toBe(true);
    });

    it("drops empty areas when no menus are present for them", () => {
      // Only provide an operations menu
      const menus: MenuItem[] = [
        { id: "overview", label: "Overview", path: "/overview", icon: "layout-dashboard", category: "Workspace" },
      ];

      const tree = NavigationTree.build({
        menus,
        plugins: [],
        pluginAreas: [COMMERCE_AREA],
        backendLoaded: true,
      });

      const areaIds = tree.areas.map((a) => a.id);
      expect(areaIds).toContain("operations");
      // Assets has no items, Commerce has no items → dropped
      expect(areaIds).not.toContain("assets");
      expect(areaIds).not.toContain("commerce");
    });

    it("enforces declared group order within areas", () => {
      // Provide Marketing before Workspace in menus array
      const menus: MenuItem[] = [
        { id: "links", label: "Links", path: "/links", icon: "link-2", category: "Marketing" },
        { id: "audit", label: "Audit", path: "/audit", icon: "scroll-text", category: "System" },
        { id: "overview", label: "Overview", path: "/overview", icon: "layout-dashboard", category: "Workspace" },
      ];

      const tree = NavigationTree.build({
        menus,
        plugins: [],
        backendLoaded: true,
      });

      const ops = tree.findArea("operations")!;
      const groupLabels = ops.groups.map((g) => g.label);
      // Workspace should precede Marketing and System according to BUILTIN_AREA_GROUPS.operations
      expect(groupLabels.indexOf("Workspace")).toBeLessThan(groupLabels.indexOf("Marketing"));
      expect(groupLabels.indexOf("Marketing")).toBeLessThan(groupLabels.indexOf("System"));
    });

    it("respects role gating for menu items", () => {
      const menus: MenuItem[] = [
        { id: "overview", label: "Overview", path: "/overview", icon: "layout-dashboard", category: "Workspace" },
        { id: "admin-only", label: "Admin Tool", path: "/admin-tool", icon: "key", category: "System", requiredRole: "admin" },
      ];

      // As member
      const memberTree = NavigationTree.build({
        menus,
        role: "member",
        isInstanceAdmin: false,
      });
      const memberItems = memberTree.areas.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => i.id)));
      expect(memberItems).not.toContain("admin-only");

      // As admin
      const adminTree = NavigationTree.build({
        menus,
        role: "admin",
        isInstanceAdmin: false,
      });
      const adminItems = adminTree.areas.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => i.id)));
      expect(adminItems).toContain("admin-only");
    });

    it("hides items owned by disabled plugins", () => {
      const plugins: PluginInfo[] = [
        {
          key: "links",
          title: "Links",
          enabled: false,
          menus: [{ id: "links", label: "Links", path: "/links", icon: "link-2", category: "Marketing" }],
        },
      ];

      const menus: MenuItem[] = [
        { id: "overview", label: "Overview", path: "/overview", icon: "layout-dashboard", category: "Workspace" },
        { id: "links", label: "Links", path: "/links", icon: "link-2", category: "Marketing" },
      ];

      const tree = NavigationTree.build({
        menus,
        plugins,
      });

      const allItems = tree.areas.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => i.path)));
      expect(allItems).not.toContain("/links");
      expect(tree.disabledPaths.has("/links")).toBe(true);
      expect(tree.disabledPlugins.has("links")).toBe(true);
    });

    it("filters Instance group from settings for non-instance admins", () => {
      const nonAdminTree = NavigationTree.build({ isInstanceAdmin: false });
      expect(nonAdminTree.settingsArea.groups.some((g) => g.label === "Instance")).toBe(false);

      const adminTree = NavigationTree.build({ isInstanceAdmin: true });
      expect(adminTree.settingsArea.groups.some((g) => g.label === "Instance")).toBe(true);
    });

    it("assembles menus into areas prioritized by declared area metadata", () => {
      const menus: MenuItem[] = [
        { id: "s3", label: "Object Storage", path: "/storage", icon: "hard-drive", category: "Storage", area: "assets" },
        { id: "pg", label: "PostgreSQL", path: "/databases", icon: "database", category: "Databases", area: "assets" },
        { id: "links", label: "Links", path: "/links", icon: "link-2", category: "Marketing", area: "operations" },
      ];

      const tree = NavigationTree.build({
        menus,
        plugins: [],
        pluginAreas: [],
        backendLoaded: true,
      });

      const assets = tree.findArea("assets");
      expect(assets).toBeDefined();
      const assetsItems = assets!.groups.flatMap((g) => g.items.map((i) => i.id));
      expect(assetsItems).toContain("s3");
      expect(assetsItems).toContain("pg");

      const ops = tree.findArea("operations");
      expect(ops).toBeDefined();
      const opsItems = ops!.groups.flatMap((g) => g.items.map((i) => i.id));
      expect(opsItems).toContain("links");
    });
  });

  describe("path & active area resolution", () => {
    const menus: MenuItem[] = [
      { id: "overview", label: "Overview", path: "/overview", icon: "layout-dashboard", category: "Workspace" },
      { id: "links", label: "Links", path: "/links", icon: "link-2", category: "Marketing" },
      { id: "domains", label: "DNS", path: "/domains", icon: "globe", category: "Network" },
    ];

    const tree = NavigationTree.build({ menus });

    it("resolves areaForPath via longest-prefix matching", () => {
      expect(tree.areaForPath("/overview")).toBe("operations");
      expect(tree.areaForPath("/links")).toBe("operations");
      expect(tree.areaForPath("/links/edit/1")).toBe("operations");
      expect(tree.areaForPath("/domains")).toBe("assets");
      expect(tree.areaForPath("/domains/records")).toBe("assets");
      expect(tree.areaForPath("/settings/general")).toBe("settings");
      expect(tree.areaForPath("/help/guide")).toBe("help");
      expect(tree.areaForPath("/unknown-route")).toBe("operations");
    });

    it("identifies settings and help paths", () => {
      expect(tree.isSettingsPath("/settings")).toBe(true);
      expect(tree.isSettingsPath("/settings/profile")).toBe(true);
      expect(tree.isSettingsPath("/overview")).toBe(false);

      expect(tree.isHelpPath("/help")).toBe(true);
      expect(tree.isHelpPath("/help/getting-started")).toBe(true);
      expect(tree.isHelpPath("/admin/help/api")).toBe(true);
      expect(tree.isHelpPath("/overview")).toBe(false);
    });

    it("resolves active area and current area", () => {
      expect(tree.resolveActiveArea("/settings/profile")).toBe("settings");
      expect(tree.resolveActiveArea("/help/quickstart")).toBe("help");
      expect(tree.resolveActiveArea("/domains")).toBe("assets");

      const opsArea = tree.getCurrentArea("operations");
      expect(opsArea.id).toBe("operations");
      const settingsArea = tree.getCurrentArea("settings");
      expect(settingsArea.id).toBe("settings");
    });
  });
});

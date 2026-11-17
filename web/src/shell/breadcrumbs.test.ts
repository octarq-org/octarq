// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { LayoutDashboard, Link2, Palette } from "lucide-react";
import type { Area } from "./areas";
import { resolveBreadcrumbs } from "./breadcrumbs";

const t = ((key: string, fallback?: string) => fallback ?? key) as unknown as Parameters<typeof resolveBreadcrumbs>[0]["t"];

const areas: Area[] = [
  {
    id: "operations",
    title: "Operations",
    subtitle: "",
    Icon: LayoutDashboard,
    groups: [
      { label: "Workspace", items: [{ id: "overview", label: "Overview", Icon: LayoutDashboard, path: "/overview" }] },
      { label: "Marketing", items: [{ id: "links", label: "Links", Icon: Link2, path: "/links" }] },
    ],
  },
];

const settingsArea: Area = {
  id: "settings",
  title: "Settings",
  subtitle: "",
  Icon: Palette,
  groups: [
    { label: "Personal", items: [{ id: "appearance", label: "Appearance", Icon: Palette, path: "/settings/appearance" }] },
  ],
};

describe("resolveBreadcrumbs", () => {
  it("resolves area / group / item for plugin and workspace pages", () => {
    expect(resolveBreadcrumbs({ pathname: "/links", areas, settingsArea, t })).toEqual([
      { label: "Operations", path: "/overview" },
      { label: "Marketing" },
      { label: "Links", path: "/links" },
    ]);
  });

  it("resolves nested subpaths to their parent route via longest prefix", () => {
    expect(resolveBreadcrumbs({ pathname: "/links/123/analytics", areas, settingsArea, t })).toEqual([
      { label: "Operations", path: "/overview" },
      { label: "Marketing" },
      { label: "Links", path: "/links" },
    ]);
  });

  it("resolves settings sub-routes cleanly", () => {
    expect(resolveBreadcrumbs({ pathname: "/settings/appearance", areas, settingsArea, t })).toEqual([
      { label: "Settings", path: "/settings/appearance" },
      { label: "Personal" },
      { label: "Appearance", path: "/settings/appearance" },
    ]);
  });
});
